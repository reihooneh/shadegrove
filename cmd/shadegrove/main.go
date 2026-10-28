// Command shadegrove plans street trees for shade from a scene file.
//
//	shadegrove scene.json          a plain-text report
//	shadegrove -json scene.json    the full result as JSON
//
// Exit codes: 0 done, 2 bad usage or input.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/reihooneh/shadegrove/engine"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	asJSON := false
	if len(args) == 2 && args[0] == "-json" {
		asJSON, args = true, args[1:]
	}
	if len(args) != 1 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stderr, "usage: shadegrove [-json] <scene.json>   (or - for standard input)")
		return 2
	}
	var r io.Reader = stdin
	if args[0] != "-" {
		info, err := os.Stat(args[0])
		if err != nil || !info.Mode().IsRegular() {
			fmt.Fprintln(stderr, "shadegrove: the scene could not be read (it must be a readable file)")
			return 2
		}
		f, err := os.Open(args[0])
		if err != nil {
			fmt.Fprintln(stderr, "shadegrove: the scene could not be read (it must be a readable file)")
			return 2
		}
		defer f.Close()
		r = f
	}
	scene, err := engine.ParseScene(r)
	if err != nil {
		var in *engine.InputError
		if errors.As(err, &in) {
			fmt.Fprintf(stderr, "shadegrove: %s\n", in.Msg)
			return 2
		}
		fmt.Fprintln(stderr, "shadegrove: the scene could not be read")
		return 2
	}
	res, err := engine.Run(scene)
	if err != nil {
		fmt.Fprintf(stderr, "shadegrove: %s\n", err)
		return 2
	}
	if asJSON {
		enc := json.NewEncoder(stdout)
		if err := enc.Encode(res); err != nil {
			fmt.Fprintln(stderr, "shadegrove: could not write the result")
			return 2
		}
		return 0
	}
	fmt.Fprint(stdout, res.Text())
	return 0
}
