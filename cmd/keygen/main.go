// Command keygen generates a static Curve25519 identity for the VPN.
//
// With -out, the base64 private key is written to the given file (mode 0600) and
// the base64 PUBLIC key is printed to stdout (handy for scripting allowlists and
// -server-key). Without -out, both keys are printed.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/AlexMelanFromRingo/vpn-demo/pkg/session"
)

func main() {
	out := flag.String("out", "", "write the private key to this file (0600); print public key to stdout")
	flag.Parse()

	id, err := session.GenerateIdentity()
	if err != nil {
		fmt.Fprintln(os.Stderr, "keygen:", err)
		os.Exit(1)
	}

	if *out == "" {
		fmt.Printf("private: %s\npublic:  %s\n", id.PrivateKeyBase64(), id.PublicKeyBase64())
		return
	}

	if err := os.WriteFile(*out, []byte(id.PrivateKeyBase64()+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "keygen:", err)
		os.Exit(1)
	}
	fmt.Println(id.PublicKeyBase64()) // stdout: public key only, for capture
	fmt.Fprintf(os.Stderr, "private key written to %s\n", *out)
}
