// Command conduit-init prepares a local Conduit quickstart: a development CA,
// a gateway server certificate, agent client certificates, and a UI login.
// It writes everything into one directory (a Docker volume in the quickstart)
// and is idempotent: existing files are kept, so restarts reuse credentials.
//
// Development use only: the CA key lives next to the certificates it signs.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tjscanlan/conduit/internal/devpki"
)

func main() {
	dir := flag.String("dir", "/conduit", "output directory")
	agents := flag.String("agents", "demo-agent,my-agent", "comma-separated agent names to issue client certificates for")
	hosts := flag.String("gateway-hosts", "gateway,localhost,127.0.0.1", "gateway certificate SANs (DNS names or IPs)")
	uiUser := flag.String("ui-user", "admin", "UI login username")
	flag.Parse()

	if err := devpki.Init(*dir, devpki.SplitList(*agents), devpki.SplitList(*hosts), *uiUser, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "conduit-init:", err)
		os.Exit(1)
	}
}
