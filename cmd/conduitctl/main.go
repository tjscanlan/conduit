// Command conduitctl runs a local Conduit with one command and helps connect
// agents to it. It needs only Docker: the quickstart's compose bundle is
// embedded, and released images are pulled.
//
//	conduitctl quickstart        start (or restart) and print how to use it
//	conduitctl status            show the services and the login again
//	conduitctl agent-cert NAME   issue a certificate for another agent
//	conduitctl doctor            diagnose a stack that isn't working
//	conduitctl down              stop, keeping data and credentials
//	conduitctl reset             stop and delete all data and credentials
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
)

// version is set at release build time (-ldflags "-X main.version=vX.Y.Z").
// `go install ...@vX.Y.Z` builds get it from the module version instead.
var version = "dev"

func init() {
	if version != "dev" {
		return
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" &&
		!isPseudoVersion(bi.Main.Version) {
		version = bi.Main.Version
	}
}

// isPseudoVersion: v0.0.0-20260101000000-abcdef123456 style versions name
// commits, not releases, so there are no images for them.
func isPseudoVersion(v string) bool {
	n := 0
	for _, c := range v {
		if c == '-' {
			n++
		}
	}
	return n >= 2
}

const usage = `conduitctl: run Conduit locally and connect agents to it.

Usage:
  conduitctl quickstart [flags]    start (or restart) the stack and print how to use it
  conduitctl status [flags]        show the services and the login
  conduitctl agent-cert NAME       issue a client certificate for agent NAME
  conduitctl doctor [flags]        check Docker, ports, the stack, and the gateway
  conduitctl down [flags]          stop the stack, keeping data and credentials
  conduitctl reset [flags]         stop and delete all data and credentials
  conduitctl version

Flags (all commands):
  -dir DIR            where the stack's files and credentials live (default ~/.conduit/quickstart)
  -ui-port N          UI port on 127.0.0.1 (default 8080)
  -gateway-port N     gateway port on 127.0.0.1 (default 4317)
  -images VERSION     image tag to run (default: this conduitctl's version)
  -no-demo            quickstart: don't start the simulated demo-agent
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	if cmd == "version" || cmd == "-version" || cmd == "--version" {
		fmt.Println("conduitctl", version)
		return
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		fmt.Print(usage)
		return
	}

	home, _ := os.UserHomeDir()
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	st := &stack{out: os.Stdout}
	fs.StringVar(&st.dir, "dir", filepath.Join(home, ".conduit", "quickstart"), "")
	fs.IntVar(&st.uiPort, "ui-port", 8080, "")
	fs.IntVar(&st.gatewayPort, "gateway-port", 4317, "")
	fs.StringVar(&st.images, "images", version, "")
	fs.BoolVar(&st.noDemo, "no-demo", false, "")
	_ = fs.Parse(args)

	var err error
	switch cmd {
	case "quickstart", "up":
		err = st.up()
	case "status":
		err = st.status()
	case "down":
		err = st.down()
	case "reset":
		err = st.reset()
	case "doctor":
		err = st.doctor()
	case "agent-cert":
		if fs.NArg() != 1 {
			err = fmt.Errorf("usage: conduitctl agent-cert NAME")
		} else {
			err = st.agentCert(fs.Arg(0))
		}
	default:
		err = fmt.Errorf("unknown command %q (run conduitctl help)", cmd)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "conduitctl:", err)
		os.Exit(1)
	}
}
