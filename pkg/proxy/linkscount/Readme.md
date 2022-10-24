remember we need two operations for kube-proxy to use the package linkscount
1. set up pod mounts for kube-proxy:  Mount the host's /proc/1 to the /host/proc directory of the kube-proxy container and the /root/kube-proxy to the /root/kube-proxy  (mainly for master)
2. install nsenter in the kube-proxy's container (mainly for master)