package main

import (
	"bufio"
	"context"
	"net"
	"os"
	"strings"
	"time"
)

// initResolver 解决 Android 上 Go 的 DNS 解析问题：
// 系统没有 /etc/resolv.conf，Go 默认会去连 [::1]:53，而 netd 的 DNS 代理
// 不是标准 socket，于是报 connection refused。这里显式给出一组候选 DNS。
//
// 顺序：用户配置 → 阿里/腾讯公共 DNS → 默认网关 → 回环。
func initResolver(custom string) {
	servers := make([]string, 0, 4)
	if s := strings.TrimSpace(custom); s != "" {
		servers = append(servers, s)
	}
	servers = append(servers, "223.5.5.5", "119.29.29.29")
	if gw := defaultGateway(); gw != "" {
		servers = append(servers, gw)
	}
	servers = append(servers, "127.0.0.1")

	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 4 * time.Second}
			var lastErr error
			for _, server := range servers {
				conn, err := dialer.DialContext(ctx, "udp", net.JoinHostPort(server, "53"))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
}

// defaultGateway 从 /proc/net/route 读默认网关（hex 小端）。
func defaultGateway() string {
	f, err := os.Open("/proc/net/route")
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 || fields[1] != "00000000" {
			continue
		}
		hex := fields[2]
		if len(hex) != 8 {
			continue
		}
		bytes := make([]byte, 4)
		for i := 0; i < 4; i++ {
			bytes[3-i] = hexByte(hex[i*2], hex[i*2+1])
		}
		return net.IPv4(bytes[0], bytes[1], bytes[2], bytes[3]).String()
	}
	return ""
}

func hexByte(hi, lo byte) byte {
	return hexVal(hi)<<4 | hexVal(lo)
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10
	}
	return 0
}
