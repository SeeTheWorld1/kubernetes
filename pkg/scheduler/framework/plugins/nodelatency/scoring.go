/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nodelatency

import (
	"context"
	"fmt"
	"net/rpc"
	"sync/atomic"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// preScoreStateKey is the key in CycleState to InterPodAffinity pre-computed data for Scoring.
const preScoreStateKey = "PreScore" + Name

type nodesSet map[string]bool

// Each key corresponds to an affinity item.
// each value is a node Set that matches the affinity item.
type affinityTermToNodesSet map[string]nodesSet

// preScoreState computed at PreScore and used at Score.
type preScoreState struct {
	// dependencies is related to the affinity of pod, which is a map
	// its key is a string representing an affinityterm, which is often a kind of pod
	// the value is a set, which contains the node name of the node where this kind of pod is located
	dependencies affinityTermToNodesSet

	// if one existing pod has an affinityterm match this pod,
	// it will save its node's name here
	nodesDependOnThisPod nodesSet

	// for one node, if there is an anti-affinity item of the pods on the node matches the incoming pod，
	// or there is at least one pod on the node matches the incoming pod's anti-affinity terms,
	// then the node will be added to badNodes.
	// It will be used to reduce the score of this node
	badNodes nodesSet

	podInfo *framework.PodInfo
	// A copy of the incoming pod's namespace labels.
	namespaceLabels labels.Set
}

// The scoreMap is used to store the scores of each node calculated in the preScore stage
type scoreMap struct {
	Scores map[string]int64
}

// Clone implements the mandatory Clone interface. We don't really copy the data since
// there is no need for that.
func (s *scoreMap) Clone() framework.StateData {
	return s
}

func (m affinityTermToNodesSet) append(other affinityTermToNodesSet) {
	for affinityTerm, onodesset := range other {
		nodesset := m[affinityTerm]
		if nodesset == nil {
			m[affinityTerm] = onodesset
			continue
		}
		for nodeName := range onodesset {
			nodesset[nodeName] = true
		}
	}
}

func (s nodesSet) append(other nodesSet) {
	for nodeName := range other {
		// Performance Comparison of Repeated Insertion and Checking the Existence of Elements
		s[nodeName] = true
	}
}

func getPreScoreState(cycleState *framework.CycleState) (*scoreMap, error) {
	c, err := cycleState.Read(preScoreStateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q from cycleState: %w", preScoreStateKey, err)
	}

	s, ok := c.(*scoreMap)
	if !ok {
		return nil, fmt.Errorf("%+v  convert to interpodaffinity.preScoreState error", c)
	}
	return s, nil
}

// Score invoked at the Score extension point.
// The "score" returned in this function is the sum of weights got from cycleState which have its topologyKey matching with the node's labels.
// it is normalized later.
// Note: the returned "score" is positive for pod-affinity, and negative for pod-antiaffinity.
func (pl *NodeLatency) Score(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	// We have obtained normalized scores for each node
	// So we just return the score of each node here
	s, err := getPreScoreState(cycleState)
	if err != nil {
		return 0, framework.AsStatus(err)
	}

	return s.Scores[nodeName], nil
}

// NormalizeScore normalizes the score for each filteredNode.
func (pl *NodeLatency) NormalizeScore(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	// because we have done Normalize in the preScore stage,, we do nothing here
	klog.Infof("Nodes final score: %v", scores)

	return nil
}

// ScoreExtensions of the Score plugin.
func (pl *NodeLatency) ScoreExtensions() framework.ScoreExtensions {
	return pl
}

// PreScore builds and writes cycle state used by Score and NormalizeScore.
func (pl *NodeLatency) PreScore(
	pCtx context.Context,
	cycleState *framework.CycleState,
	pod *v1.Pod,
	nodes []*v1.Node,
) *framework.Status {
	if len(nodes) == 0 {
		// No nodes to score.
		return nil
	}

	if pl.sharedLister == nil {
		return framework.NewStatus(framework.Error, "empty shared lister in InterPodAffinity PreScore")
	}

	affinity := pod.Spec.Affinity
	hasPreferredAffinityConstraints := affinity != nil && affinity.PodAffinity != nil && len(affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0
	hasPreferredAntiAffinityConstraints := affinity != nil && affinity.PodAntiAffinity != nil && len(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution) > 0

	// Unless the pod being scheduled has preferred affinity terms, we only
	// need to process nodes hosting pods with affinity.
	var allNodes []*framework.NodeInfo
	var err error
	if hasPreferredAffinityConstraints || hasPreferredAntiAffinityConstraints {
		allNodes, err = pl.sharedLister.NodeInfos().List()
		if err != nil {
			return framework.AsStatus(fmt.Errorf("failed to get all nodes from shared lister: %w", err))
		}
	} else {
		allNodes, err = pl.sharedLister.NodeInfos().HavePodsWithAffinityList()
		if err != nil {
			return framework.AsStatus(fmt.Errorf("failed to get pods with affinity list: %w", err))
		}
	}

	myState := &preScoreState{
		dependencies:         make(affinityTermToNodesSet),
		nodesDependOnThisPod: make(nodesSet),
		badNodes:             make(nodesSet),
	}

	myState.podInfo = framework.NewPodInfo(pod)

	if myState.podInfo.ParseError != nil {
		// Ideally we never reach here, because errors will be caught by PreFilter
		return framework.AsStatus(fmt.Errorf("failed to parse pod: %w", myState.podInfo.ParseError))
	}

	for i := range myState.podInfo.PreferredAffinityTerms {
		if err := pl.mergeAffinityTermNamespacesIfNotEmpty(&myState.podInfo.PreferredAffinityTerms[i].AffinityTerm); err != nil {
			return framework.AsStatus(fmt.Errorf("updating PreferredAffinityTerms: %w", err))
		}
	}
	for i := range myState.podInfo.PreferredAntiAffinityTerms {
		if err := pl.mergeAffinityTermNamespacesIfNotEmpty(&myState.podInfo.PreferredAntiAffinityTerms[i].AffinityTerm); err != nil {
			return framework.AsStatus(fmt.Errorf("updating PreferredAntiAffinityTerms: %w", err))
		}
	}
	myState.namespaceLabels = GetNamespaceLabelsSnapshot(pod.Namespace, pl.nsLister)

	// corresponds to preScoreState.dependencies
	aTTNodesSets := make([]affinityTermToNodesSet, len(allNodes))
	aTTindex := int32(-1)
	// corresponds to preScoreState.nodesDependOnThisPod
	nodesSetS := make([]nodesSet, len(allNodes))
	nSSindex := int32(-1)
	// corresponds to preScoreState.badNodes
	badnodes := make([]nodesSet, len(allNodes))
	bdnindex := int32(-1)

	processNode := func(i int) {
		nodeInfo := allNodes[i]
		if nodeInfo.Node() == nil {
			return
		}
		// Unless the pod being scheduled has preferred affinity terms, we only
		// need to process pods with affinity in the node.
		podsToProcess := nodeInfo.PodsWithAffinity
		if hasPreferredAffinityConstraints || hasPreferredAntiAffinityConstraints {
			// We need to process all the pods.
			podsToProcess = nodeInfo.Pods
		}

		aTTNodesSet := make(affinityTermToNodesSet)
		nodesset := make(nodesSet)
		bdnsSet := make(nodesSet)
		for _, existingPod := range podsToProcess {
			pl.processExistingPod(myState, existingPod, nodeInfo, pod, aTTNodesSet, nodesset, bdnsSet)
		}
		if len(aTTNodesSet) > 0 {
			aTTNodesSets[atomic.AddInt32(&aTTindex, 1)] = aTTNodesSet
		}
		if len(nodesset) > 0 {
			nodesSetS[atomic.AddInt32(&nSSindex, 1)] = nodesset
		}
		if len(bdnsSet) > 0 {
			badnodes[atomic.AddInt32(&bdnindex, 1)] = bdnsSet
		}
	}
	pl.parallelizer.Until(pCtx, len(allNodes), processNode)

	for i := 0; i <= int(aTTindex); i++ {
		myState.dependencies.append(aTTNodesSets[i])
	}

	for i := 0; i <= int(nSSindex); i++ {
		myState.nodesDependOnThisPod.append(nodesSetS[i])
	}

	for i := 0; i <= int(bdnindex); i++ {
		myState.badNodes.append(badnodes[i])
	}

	masterIp := pl.getMasterIp()

	// Calculate the normalized score for each node
	scoresMap := &scoreMap{
		Scores: make(map[string]int64),
	}
	nodesName := make([]string, 0)
	for _, node := range nodes {
		nodesName = append(nodesName, node.Name)
	}
	getNodeScores(nodesName, myState, scoresMap, masterIp)
	cycleState.Write(preScoreStateKey, scoresMap)

	return nil
}

func (m affinityTermToNodesSet) podAffinityTermToExistingPods(term *framework.AffinityTerm, weight int32, pod *v1.Pod, nsLabels labels.Set, node *v1.Node) {
	if term.Matches(pod, nsLabels) {
		if m[term.Selector.String()] == nil {
			m[term.Selector.String()] = make(map[string]bool)
		}
		m[term.Selector.String()][node.Name] = true
	}
}

func (m affinityTermToNodesSet) podAffinityTermsToExistingPods(terms []framework.WeightedAffinityTerm, pod *v1.Pod, nsLabels labels.Set, node *v1.Node) {
	for _, term := range terms {
		m.podAffinityTermToExistingPods(&term.AffinityTerm, term.Weight, pod, nsLabels, node)
	}
}

func (s nodesSet) processTerm(term *framework.AffinityTerm, weight int32, pod *v1.Pod, nsLabels labels.Set, node *v1.Node) {
	if term.Matches(pod, nsLabels) {
		s[node.Name] = true
	}
}

func (s nodesSet) processTerms(terms []framework.WeightedAffinityTerm, pod *v1.Pod, nsLabels labels.Set, node *v1.Node) {
	for _, term := range terms {
		s.processTerm(&term.AffinityTerm, term.Weight, pod, nsLabels, node)
	}
}

// this function calculate each nodes Sets we needed.
func (pl *NodeLatency) processExistingPod(
	state *preScoreState,
	existingPod *framework.PodInfo,
	existingPodNodeInfo *framework.NodeInfo,
	incomingPod *v1.Pod,
	aTTNodesSet affinityTermToNodesSet,
	nodesset nodesSet,
	bdnsSet nodesSet,
) {
	existingPodNode := existingPodNodeInfo.Node()
	if len(existingPodNode.Labels) == 0 {
		return
	}

	// For every soft pod affinity term of <pod>, if <existingPod> matches the term,
	// Add the node of the existingPod to this term of dependencies
	// Note that the incoming pod's terms have the namespaceSelector merged into the namespaces, and so
	// here we don't lookup the existing pod's namespace labels, hence passing nil for nsLabels.
	aTTNodesSet.podAffinityTermsToExistingPods(state.podInfo.PreferredAffinityTerms, existingPod.Pod, nil, existingPodNode)

	// For every soft pod anti-affinity term of <pod>, if <existingPod> matches the term,
	// just add the node to badNodes
	// Note that the incoming pod's terms have the namespaceSelector merged into the namespaces, and so
	// here we don't lookup the existing pod's namespace labels, hence passing nil for nsLabels.
	bdnsSet.processTerms(state.podInfo.PreferredAntiAffinityTerms, existingPod.Pod, nil, existingPodNode)

	// For every soft pod anti-affinity term of <existingPod>, if <pod> matches the term,
	// just add the node to badNodes
	bdnsSet.processTerms(existingPod.PreferredAntiAffinityTerms, incomingPod, state.namespaceLabels, existingPodNode)

	// For every soft pod affinity term of <existingPod>, if <pod> matches the term,
	// Add the node of the existingPod to nodesDependOnThisPod
	nodesset.processTerms(existingPod.PreferredAffinityTerms, incomingPod, state.namespaceLabels, existingPodNode)
}

type Args struct {
	CandidateNodes       []string
	Dependencies         [][]string
	NodesDependOnThisPod []string
}

// getNodeScores calls the corresponding rpc function to score nodes
// according to the affinity between pods and the network distance between nodes
func getNodeScores(nodesName []string, state *preScoreState, scoresMap *scoreMap, masterIp string) {
	// establish tcp link
	masterIp += ":9091"
	client, err := rpc.Dial("tcp", masterIp)
	if err != nil {
		klog.Errorf("nodelatency/scoring.go getNodeScores() error: call rpc function fail!")
	}

	// Synchronous call
	nodesDependOnThisPod := make([]string, 0)
	for name := range state.nodesDependOnThisPod {
		nodesDependOnThisPod = append(nodesDependOnThisPod, name)
	}
	dependencies := make([][]string, 0)
	for _, nodes := range state.dependencies {
		nodesList := make([]string, 0)
		for name := range nodes {
			nodesList = append(nodesList, name)
		}
		dependencies = append(dependencies, nodesList)
	}
	args := &Args{
		CandidateNodes:       nodesName,
		NodesDependOnThisPod: nodesDependOnThisPod,
		Dependencies:         dependencies,
	}

	err = client.Call("ServiceA.GetNodesScores", args, scoresMap)
	if err != nil {
		klog.Errorf("nodelatency/scoring.go getNodeScores() : ServiceA.GetNodesScores error: %v", err)
	}
	// If the node is included in badnodes,
	// the score is divided by 2
	for name := range state.badNodes {
		scoresMap.Scores[name] /= 2
	}
	client.Close()
}

// get master Ip
func (pl *NodeLatency) getMasterIp() string {
	allNodes, err := pl.sharedLister.NodeInfos().List()
	if err != nil {
		klog.Errorf("nodelatency/scoring.go getMasterIp() error: get nodes information fail!")
	}

	for _, nodeInfo := range allNodes {
		if _, ok := nodeInfo.Node().Labels["node-role.kubernetes.io/control-plane"]; ok {
			return nodeInfo.Node().Status.Addresses[0].Address
		}
		if _, ok := nodeInfo.Node().Labels["node-role.kubernetes.io/master"]; ok {
			return nodeInfo.Node().Status.Addresses[0].Address
		}
	}
	klog.Errorf("nodelatency/scoring.go getMasterIp() error: can't find master node!")
	return ""
}
