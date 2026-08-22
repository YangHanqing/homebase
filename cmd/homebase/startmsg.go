package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/yanghanqing/homebase/internal/devices"
	"github.com/yanghanqing/homebase/internal/listen"
)

func printStartSuccess(r *resolved) int {
	zh := uiChinese()
	port := r.res.Port
	local := loopbackURL(port)

	if zh {
		fmt.Println("Homebase 已启动。")
		fmt.Println()
		fmt.Println("本机打开：")
		fmt.Printf("  %s\n", local)
	} else {
		fmt.Println("Homebase is running.")
		fmt.Println()
		fmt.Println("On this machine:")
		fmt.Printf("  %s\n", local)
	}

	switch {
	case r.cfg.Access == "lan" || r.res.Unspecified:
		printLANAddrs(r.res, zh)
	case r.res.Trusted && !r.res.Loopback:
		ts := strings.TrimRight(baseURL(r.res), "/")
		if zh {
			fmt.Println()
			fmt.Println("Tailscale 网络上：")
			fmt.Printf("  %s\n", ts)
			fmt.Println("  新设备配对：  homebase pair")
		} else {
			fmt.Println()
			fmt.Println("On your Tailscale network:")
			fmt.Printf("  %s\n", ts)
			fmt.Println("  Pair a new device:  homebase pair")
		}
	}

	fmt.Println()
	if zh {
		fmt.Println("Homebase 是明文 HTTP。别的设备请走 Tailscale 或 WireGuard。")
	} else {
		fmt.Println("Homebase is plain HTTP. Other devices should connect over Tailscale or WireGuard.")
	}

	if r.res.NeedsAuth() {
		maybeMintPair(r)
	}
	return 0
}

func printLANAddrs(res listen.Result, zh bool) {
	ips := listen.PrivateIPv4s()
	if zh {
		fmt.Println()
		fmt.Println("局域网（明文 HTTP，安全取决于该网络是否可信）：")
	} else {
		fmt.Println()
		fmt.Println("On the local network (plain HTTP — security depends on whether that network is trusted):")
	}
	if len(ips) == 0 {
		fmt.Printf("  %s\n", strings.TrimRight(baseURL(res), "/"))
		return
	}
	for _, ip := range ips {
		fmt.Printf("  http://%s:%d\n", ip, res.Port)
	}
	if zh {
		fmt.Println("  新设备配对：  homebase pair")
	} else {
		fmt.Println("  Pair a new device:  homebase pair")
	}
}

func maybeMintPair(r *resolved) {
	list, err := r.dev.List()
	if err != nil || len(list) > 0 {
		return
	}
	token, expires, err := r.dev.Mint(time.Now())
	if err != nil {
		return
	}
	fmt.Println()
	if uiChinese() {
		fmt.Println("还没有配对设备。把下面的链接在要授权的设备上打开：")
	} else {
		fmt.Println("No devices paired yet. Open this link on the device you want to authorize:")
	}
	printPairLinks(pairURLs(r.res, token), expires.Format("15:04:05"), devices.TokenTTL.Minutes())
	fmt.Println()
}
