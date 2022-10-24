package proxy

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	utilfeature "k8s.io/apiserver/pkg/util/feature"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/features"
	"k8s.io/kubernetes/pkg/proxy/latency"
)

// get all available endpoints with latency
func GetEndpointsWithLatency(endpoints []Endpoint, hostname string) (allReachableEndpoints []Endpoint, endpointsLatency []time.Duration, hasAnyEndpoints bool) {
	// get available endpoints
	allReachableEndpoints = filterEndpoints(endpoints, func(ep Endpoint) bool {
		return ep.IsReady()
	})

	// if there are 0 ready endpoints, we can try to fallback to any terminating endpoints that are ready.
	if len(allReachableEndpoints) == 0 && utilfeature.DefaultFeatureGate.Enabled(features.ProxyTerminatingEndpoints) {
		allReachableEndpoints = filterEndpoints(endpoints, func(ep Endpoint) bool {
			if ep.IsServing() && ep.IsTerminating() {
				return true
			}

			return false
		})
	}
	// If there are any Ready endpoints anywhere in the cluster, we are
	// guaranteed to get one in clusterEndpoints.
	if len(allReachableEndpoints) > 0 {
		hasAnyEndpoints = true
	}

	// estimates latency to nodes if the node's latency is not recorded
	wg := new(sync.WaitGroup)
	var nodeName string
	t := 20 * time.Microsecond // if the endpoint is on this node, we set its latency to 0.02ms by default
	for _, dst := range allReachableEndpoints {
		nodeName = dst.GetNodeName()
		latency.NodeLatency.Lock()
		// this operation didn't consider concurrent read
		_, ok := latency.NodeLatency.TimeMap[nodeName]
		if !ok {
			if nodeName == hostname {
				latency.NodeLatency.TimeMap[dst.GetNodeName()] = t
				latency.NodeLatency.Unlock()
				continue
			}

			// -1 means the latency is not obtained by ping, there may be a bug in this operation
			// we guess and believe the endpoints passed is reachable
			latency.NodeLatency.TimeMap[dst.GetNodeName()] = -1
			latency.NodeLatency.Unlock()
			wg.Add(1)

			// we use one endpoint's latency to replace the latency of the endpoint's node
			go GetNodeLatency(wg, dst.IP(), dst.GetNodeName())
		} else {
			latency.NodeLatency.Unlock()
		}
	}
	// make sure we get the latency per node
	wg.Wait()

	// check if has links file to adjust the endpoints' probability
	_, err := os.Stat("/root/kube-proxy/links") //Note the pathname
	if err == nil {
		// First create file 0 to represent the lock
		fTmp, err := os.OpenFile("/tmp/0", os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			klog.Errorf("proxy traffic GetEndpointsWithLatency create file 0 error: %v", err)
		}
		fTmp.Close()

		// check whether directory 1 exists，wait if it exists
		for {
			_, err := os.Stat("/tmp/1")
			if err != nil {
				// no lock dir 1, break
				break
			}
			time.Sleep(time.Millisecond * 100)
		}

		// first copy /root/kube-proxy/links to /tmp/links
		// then read from /tmp/links
		cmd := exec.Command("cp", "/root/kube-proxy/links", "/tmp/")
		if err = cmd.Run(); err != nil {
			klog.Errorf("proxy traffic GetEndpointsWithLatency failed to copy /root/kube-proxy/links to /tmp/links: %v", err)
		}

		// delete the lock file 0 after the copy of file links
		err = os.Remove("/tmp/0")
		if err != nil {
			klog.Errorf("proxy traffic GetEndpointsWithLatency remove lock file 0 error: %v", err)
		}

		// get the endpoints' links
		tmpvar := make([]int64, 0) // temp var use to record links
		endpointLinks := getEndpointLinks()
		latency.NodeLatency.RLock()
		for _, ep := range allReachableEndpoints {
			s := int64(0)
			if t, ok := endpointLinks[ep.IP()]; ok {
				s = t
			} else {
				s = 3 //The default value is set to 3 by us
			}
			tmpvar = append(tmpvar, s)
			endpointsLatency = append(endpointsLatency, time.Duration(s)*latency.NodeLatency.TimeMap[ep.GetNodeName()])
		}
		klog.Infof("svc endpoints links are: %v\n", tmpvar)
		klog.Infof("svc endpoints weighted latency are: %v\n", endpointsLatency)
		latency.NodeLatency.RUnlock()
	} else {
		// get the latency of each endpoint
		latency.NodeLatency.RLock()
		for _, ep := range allReachableEndpoints {
			endpointsLatency = append(endpointsLatency, latency.NodeLatency.TimeMap[ep.GetNodeName()])
		}
		latency.NodeLatency.RUnlock()
	}

	return
}

func GetNodeLatency(wg *sync.WaitGroup, dst, nodeName string) {
	defer wg.Done()
	latency.GetNodeLatency(dst, nodeName)
}

// get the links of correspond endpoint
func getEndpointLinks() map[string]int64 {
	linksFile, err := os.Open("/tmp/links")
	if err != nil {
		klog.Errorf("proxy traffic getEndpointLinks open file error: %v", err)
	}
	defer linksFile.Close()

	endpointLinks := make(map[string]int64)
	linksReader := bufio.NewReader(linksFile)
	for {
		links, err := linksReader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			klog.Errorf("proxy traffic getEndpointLinks read file error: %v", err)
		}
		ipAndLinks := strings.Split(links, " ")
		linksInt64, err := strconv.ParseInt(ipAndLinks[1][:len(ipAndLinks[1])-1], 10, 64) // the last character of the string is "\n" so it needs to be processed
		if err != nil {
			klog.Errorf("proxy traffic getEndpointLinks string to int64 error: %v", err)
		}
		endpointLinks[ipAndLinks[0]] = linksInt64
	}
	return endpointLinks
}
