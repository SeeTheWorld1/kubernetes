package main

import (
	"fmt"

	"k8s.io/klog/v2"
)

// func main() {
// 	testcases := []struct {
// 		name       string
// 		hostname   string
// 		pteEnabled bool
// 		endpoints  []proxy.Endpoint

// 		allEndpoints sets.String
// 		has          bool
// 	}{{
// 		name:     "hints enabled, hints annotation == auto",
// 		hostname: "master",
// 		endpoints: []proxy.Endpoint{
// 			&proxy.BaseEndpointInfo{Endpoint: "127.0.0.1:80", Serving: true, Terminating: true, NodeName: "localhost"},
// 			&proxy.BaseEndpointInfo{Endpoint: "110.242.68.3:80", Serving: true, Terminating: true, NodeName: "baidu"},
// 			&proxy.BaseEndpointInfo{Endpoint: "11.160.137.48:80", Serving: true, Terminating: true, NodeName: "master"},
// 			&proxy.BaseEndpointInfo{Endpoint: "11.158.187.14:80", Serving: true, Terminating: true, NodeName: "brother"},
// 		},
// 		allEndpoints: sets.NewString("127.0.0.1:80", "110.242.68.3:80", "11.160.137.48:80", "11.158.187.14:80"),
// 		has:          true,
// 	}}

// 	for _, tc := range testcases {
// 		defer setFeatureGateDuringTest(utilfeature.DefaultFeatureGate, features.ProxyTerminatingEndpoints, true)()

// 		//	start := time.Now()
// 		allLocallyReachableEndpoints, endpointsLatency, hasEndpoints := proxy.GetEndpointsWithLatency(tc.endpoints, tc.hostname)
// 		//	fmt.Print(time.Since(start))
// 		if tc.has != hasEndpoints {
// 			log.Fatalf("expected hasAnyEndpoints=%v, got %v", tc.has, hasEndpoints)
// 		}
// 		err := checkExpectedEndpoints(tc.allEndpoints, allLocallyReachableEndpoints)
// 		if err != nil {
// 			log.Fatalf("error with all endpoints: %v", err)
// 		} else {
// 			fmt.Println(endpointsLatency)
// 			probability := iptables.LatencyToProbability(endpointsLatency)
// 			probabilityString := iptables.ProbabilityFloatToString(probability)
// 			fmt.Println(len(probability), " ", len(probabilityString))
// 			fmt.Println(probabilityString)
// 		}
// 	}
// }

// func checkExpectedEndpoints(expected sets.String, actual []proxy.Endpoint) error {
// 	var errs []error

// 	expectedCopy := sets.NewString(expected.UnsortedList()...)
// 	for _, ep := range actual {
// 		if !expectedCopy.Has(ep.String()) {
// 			errs = append(errs, fmt.Errorf("unexpected endpoint %v", ep))
// 		}
// 		expectedCopy.Delete(ep.String())
// 	}
// 	if len(expectedCopy) > 0 {
// 		errs = append(errs, fmt.Errorf("missing endpoints %v", expectedCopy.UnsortedList()))
// 	}

// 	return kerrors.NewAggregate(errs)
// }

// func setFeatureGateDuringTest(gate featuregate.FeatureGate, f featuregate.Feature, value bool) func() {
// 	originalValue := gate.Enabled(f)

// 	if err := gate.(featuregate.MutableFeatureGate).Set(fmt.Sprintf("%s=%v", f, value)); err != nil {
// 		log.Fatalf("error setting %s=%v: %v", f, value, err)
// 	}

// 	return func() {
// 		if err := gate.(featuregate.MutableFeatureGate).Set(fmt.Sprintf("%s=%v", f, originalValue)); err != nil {
// 			log.Fatalf("error restoring %s=%v: %v", f, originalValue, err)
// 		}
// 	}
// }

func main() {
	klog.Errorf("this is Errorf\n")
	fmt.Print("yes! It won't cause the program exit\n")
}
