To use icmp, you need to adjust the kernel parameter net.ipv4.ping_group_range
You must enable it by setting sudo sysctl -w net.ipv4.ping_group_range="0   2147483647"


