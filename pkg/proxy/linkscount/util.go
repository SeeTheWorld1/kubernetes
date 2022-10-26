package linkscount

// get ip's prefix, num is the number of bytes will get
func getIpPrefix(ip string, num int) string {
	i := 0
	for ; i < len(ip); i++ {
		if ip[i] == '.' {
			num--
			if num == 0 {
				return ip[:i]
			}
		}
	}
	return ip
}

func deletePrefixSpaces(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			return s[i:]
		}
	}
	return s
}

// pod网段的问题，一定是16或24吗？
// 没考虑过ipv6的情况
