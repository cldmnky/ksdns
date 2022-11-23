/*
Copyright © 2022 cldmnky <magnus@cloudmonkey.org>
*/
package main

import (
	"github.com/cldmnky/ksdns/cmd/zupd/cmd"
	_ "github.com/cldmnky/ksdns/pkg/zupd/core/plugin" // Plug in CoreDNS.
)

func main() {
	cmd.Execute()
}
