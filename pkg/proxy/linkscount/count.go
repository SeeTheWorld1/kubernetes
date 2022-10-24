// this package will be used by master and it must use the kubectl tools
// make sure the kubectl is available
package linkscount

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"k8s.io/klog/v2"
)

func InitLinksCount() {
	//createDirectoryForTransferData()
	getPodCidr()

	linksCount()
}

// Create the directory for data transfer between the kube-proxy container and the host
// mainly used by master
// func createDirectoryForTransferData() {
// 	// 补充一个逻辑，已存在则跳过
// 	createFileStr := "mkdir " + hostFile
// 	cmd, err := Config.Command("/bin/sh", "-c", createFileStr)
// 	if err != nil {
// 		log.Fatalf("proxy/linkscount/count.go createDirectoryForTransferData() failed to build  command : %v", err)
// 	}

// 	if err := cmd.Run(); err != nil {
// 		log.Fatalf("proxy/linkscount/count.go createDirectoryForTransferData() failed to call cmd.Run(): %v", err)
// 	}
// }

var podCidr string

// get the pod cidr
func getPodCidr() {
	getPodCidrCmdStr := "kubectl get ippool -o yaml | grep cidr"
	cmd, err := Config.Command("/bin/sh", "-c", getPodCidrCmdStr)
	if err != nil {
		klog.Fatalf("proxy/linkscount/count.go getPodCidr() failed to build nsenter command : %v", err)
	}

	var stdout, stderr bytes.Buffer

	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	cmd.Env = append(cmd.Env, KUBECONFIG)

	if err := cmd.Run(); err != nil {
		klog.Fatalf("proxy/linkscount/count.go getPodCidr() failed to call cmd.Run(): %v", err)
	}

	strs := strings.Split(stdout.String(), " ")
	ipAndMask := strings.Split(strs[len(strs)-1], "/")
	ip := ipAndMask[0]
	mask := ipAndMask[1]
	mask = mask[:len(mask)-1]
	if mask == "16" {
		podCidr = getIpPrefix(ip, 2)
	}
	if mask == "24" {
		podCidr = getIpPrefix(ip, 3)
	}

	klog.Infof("out:\n%s\nerr:\n%s", podCidr, stderr.String())
}

// get pods' name
func getPodsName(podNameIpMap map[string]string) {
	// output the result to the specified file podNameIp
	// The ">" operation will reset the contents of the file
	getPodsNameStr := `kubectl get pods -A -o=jsonpath='{range .items[*]}{.status.podIP}{"\t"}{.metadata.namespace}{"/"}{.metadata.name}{"\n"}{end}' | grep ` + podCidr + ` | grep -v kube-system` + ` > ` + podNameIp
	cmd, err := Config.Command("/bin/sh", "-c", getPodsNameStr)
	if err != nil {
		klog.Fatalf("proxy/linkscount/count.go getPodsName() failed to build nsenter command : %v", err)
	}
	var stderr bytes.Buffer
	cmd.Env = append(cmd.Env, KUBECONFIG)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Here, because if grep has no result, the nsenter will also report an error
		klog.Errorf("proxy/linkscount/count.go getPodsName() failed to call cmd.Run(): %v  stderr: %s", err, stderr.String())
	}

	// save the result from the file to the podNameIpMap map
	input, err := os.Open(podNameIp)
	if err != nil {
		// if the above operation failed, this step may fail because there is no corresponding file
		klog.Errorf("kube-proxy linkscount error: getPodsName() open file failed!")
	} else {
		defer input.Close()
		inputReader := bufio.NewReader(input)
		for {
			str, err := inputReader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				klog.Fatalf("kube-proxy linkscount error: getPodsName() read file wrong!")
			}
			ipAndName := strings.Split(str, "\t")
			podNameIpMap[ipAndName[0]] = ipAndName[1][:len(ipAndName[1])-1] // now we're not sure whether the whitespace will result in some errors
		}
	}
}

// getKubeproxyPodname() get the name of the kube-proxy pod which needs the file flush iptables
func getKubeproxyPodname() []string {
	var stdout, stderr bytes.Buffer
	getKubeproxyPodnameStr := `kubectl get pods -A -o=jsonpath='{range .items[*]}{.metadata.namespace}{"/"}{.metadata.name}{"\n"}{end}' | grep kube-proxy`
	cmd, err := Config.Command("/bin/sh", "-c", getKubeproxyPodnameStr)
	if err != nil {
		klog.Fatalf("kube-proxy linkscount getKubeproxyPodname() failed to build nsenter command : %v", err)
	}
	// please consider the capacity of bytes.Buffer
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout
	cmd.Env = append(cmd.Env, KUBECONFIG)
	if err := cmd.Run(); err != nil {
		klog.Fatalf("kube-proxy linkscount getKubeproxyPodname() failed to call cmd.Run(): %v", err)
	}
	return strings.Split(stdout.String(), "\n")
}

// linksCount() will finally get the number of links per and write them to the links file
// then the links file will be copied to all kube-proxy pods
func linksCount() {
	for {
		// podNameIpMap is used to save podip and podname
		// podname is in the form of podnamespace/podname
		podNameIpMap := make(map[string]string)
		getPodsName(podNameIpMap)
		kubeproxyPods := getKubeproxyPodname()
		kubeproxyPods = kubeproxyPods[:len(kubeproxyPods)-1] // remove the last "" string

		// write the number of links per pod to a specific file when it is necessary to flush iptables
		outputFile, err := os.OpenFile(hostFile+"/links", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			klog.Fatalf("kube-proxy linkscount error: linksCount() create file failed!")
		}
		outputWriter := bufio.NewWriter(outputFile)
		for podIp, podName := range podNameIpMap {
			podNameT := strings.Split(podName, "/")
			podnamespace, podname := podNameT[0], podNameT[1]
			podLinksStr := `kubectl exec -n ` + podnamespace + ` ` + podname + ` --  wc -l /proc/net/tcp  /proc/net/udp`
			cmd, err := Config.Command("/bin/sh", "-c", podLinksStr)
			if err != nil {
				klog.Fatalf("failed to build nsenter command : %v", err)
			}
			var stdout, stderr bytes.Buffer
			cmd.Stderr = &stderr
			cmd.Stdout = &stdout
			cmd.Env = append(cmd.Env, KUBECONFIG)
			if err := cmd.Run(); err != nil {
				klog.Errorf("linksCount podNameIpMap failed to call cmd.Run(): %v", err)
				continue
			}

			linksNumTemp := strings.Split(stdout.String(), "\n")
			linksNum := strings.Split(linksNumTemp[2], " ")[0]
			outputWriter.WriteString(podIp + " " + linksNum + "\n")
			outputWriter.Flush()
			//log.Printf("out:\n%s\nerr:\n%s", linksNum, stderr.String())
		}
		outputFile.Close()

		wg := new(sync.WaitGroup)
		// copy the links file to all kube-proxy pods
		for _, kubeproxyPod := range kubeproxyPods {
			wg.Add(1)
			go copyLinksToKubeproxyPod(kubeproxyPod, wg)
		}
		wg.Wait()
		//klog.InfoS("links count ok!\n")
		time.Sleep(10 * time.Second)
	}
}

// copy the links file to all kube-proxy pods
func copyLinksToKubeproxyPod(kubeproxyPod string, wg *sync.WaitGroup) {
	defer wg.Done()
	// first create the directory named "1" in the specific kube-proxy pod
	// Its role is to operate the links file as a lock to avoid read and write conflicts
	kubeproxyPodNameT := strings.Split(kubeproxyPod, "/")
	kubeproxyPodnamespace, kubeproxyPodname := kubeproxyPodNameT[0], kubeproxyPodNameT[1]
	createFileLockStr := `kubectl exec -n ` + kubeproxyPodnamespace + ` ` + kubeproxyPodname + ` -- mkdir /tmp/1` // generally mkdir and rm operations will happen together
	cmd, err := Config.Command("/bin/sh", "-c", createFileLockStr)
	if err != nil {
		klog.Fatalf("count.go line 187 failed to build nsenter command : %v", err)
	}
	cmd.Env = append(cmd.Env, KUBECONFIG)
	if err = cmd.Run(); err != nil {
		klog.Fatalf("count.go copyLinksToKubeproxyPod() createFileLock failed, failed to call cmd.Run(): %v", err)
	}

	//Check whether the directory 0 exists, the directory 0 was created by kube-proxy itself, indicating that it wants to read the links file
	// If so, wait for 1 second.
	// The 1-second waiting time gives kube-proxy enough time to copy the links file to a specified directory and avoids deadlocks
	checkIfDirectory0Exist := `kubectl exec -n ` + kubeproxyPodnamespace + ` ` + kubeproxyPodname + ` -- ls /tmp | grep 0`
	cmd, err = Config.Command("/bin/sh", "-c", checkIfDirectory0Exist)
	if err != nil {
		klog.Fatalf("count.go line 197 failed to build nsenter command : %v", err)
	}
	var stdout bytes.Buffer
	cmd.Env = append(cmd.Env, KUBECONFIG)
	cmd.Stdout = &stdout
	if err = cmd.Run(); err == nil { // if directory 0 doesn't exist, err will != nil
		time.Sleep(time.Second * 1)
	} else {
		// Maybe the kubeproxy pod no longer exists
		klog.Errorf("count.go line 208  directory 0 doesn't exist failed to call cmd.Run(): %v", err)
	}

	// do the actual operation to copy the links file to all kube-proxy pods
	cpLinksStr := `kubectl cp ` + hostFile + "/links" + ` ` + kubeproxyPod + `:` + pathForFileLinks
	cmd, err = Config.Command("/bin/sh", "-c", cpLinksStr)
	if err != nil {
		klog.Fatalf("count.go line 217 failed to build nsenter command : %v", err)
	}
	cmd.Env = append(cmd.Env, KUBECONFIG)
	if err = cmd.Run(); err != nil {
		klog.Errorf("count.go line 221 failed to call cmd.Run(): %v", err)
	}

	// delete the directory 1 to release lock
	deleteFileLockStr := `kubectl exec -n ` + kubeproxyPodnamespace + ` ` + kubeproxyPodname + ` -- rm -rf /tmp/1`
	cmd, err = Config.Command("/bin/sh", "-c", deleteFileLockStr)
	if err != nil {
		klog.Fatalf("count.go line 228 failed to build nsenter command : %v", err)
	}
	cmd.Env = append(cmd.Env, KUBECONFIG)
	if err = cmd.Run(); err != nil {
		klog.Fatalf("count.go line 232 failed to call cmd.Run(): %v", err)
	}
}

// 需要考虑名字空间的因素
// kube-system下的pod会执行出错
// flush的时机
// copy the file to these other kube-proxy（拷贝回去的pod是kube-proxy!）
// 在加锁的代码中起一个goroutine会怎样
// the home dir of kube-proxy is /root
// 要解决对links文件的读、拷贝冲突问题
