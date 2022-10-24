package linkscount

import "k8s.io/kubernetes/pkg/proxy/linkscount/nsenter"

// this file is used to define global variables for package linkscount
const KUBECONFIG = "KUBECONFIG=/etc/kubernetes/admin.conf"
const podNameIp = "/root/kube-proxy/podNameIp" // the file in host and container to save Pod's name and the the correspond ip
const hostFile = "/root/kube-proxy"
const pathForFileLinks = "/root"

var Config = &nsenter.Config{
	MountFile: "/host/proc/ns/mnt",
	Mount:     true, // Execute into mount namespace
	// UTS:   true,
	// PID:   true,
	// IPC:   true,
	// Net:   true,
	// Target: 1, // Enter into PID 1 (init) namespace
}
