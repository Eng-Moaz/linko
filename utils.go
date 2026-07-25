package main

import (
	"fmt"
	"net"
)

func redactIP(address string) string{
	host, _ , err := net.SplitHostPort(address)
	if err != nil{
		return address
	}

	ip := net.ParseIP(host)
	if ip4 := ip.To4() ; ip4 != nil{
		return fmt.Sprintf("%d.%d.%d.x", ip4[0], ip4[1], ip4[2])
	}else{
		return address
	}
}
