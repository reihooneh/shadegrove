//go:build js && wasm

// The WebAssembly build: exposes shadegrove.plan(sceneJSON) to the web page. The page passes a
// string and gets a string back; no JavaScript objects cross the boundary, so nothing the page
// holds can be reached from here.
package main

import (
	"encoding/json"
	"errors"
	"strings"
	"syscall/js"

	"github.com/reihooneh/shadegrove/engine"
)

func reply(v any) js.Value {
	data, err := json.Marshal(v)
	if err != nil {
		return js.ValueOf(`{"error":"the result could not be encoded"}`)
	}
	return js.ValueOf(string(data))
}

func plan(_ js.Value, args []js.Value) (out any) {
	defer func() {
		if recover() != nil {
			out = reply(map[string]string{"error": "the planner hit an internal problem with this scene"})
		}
	}()
	if len(args) != 1 || args[0].Type() != js.TypeString {
		return reply(map[string]string{"error": "plan takes one JSON string"})
	}
	text := args[0].String()
	if len(text) > engine.MaxSceneBytes {
		return reply(map[string]string{"error": "the scene is larger than 256 KB"})
	}
	scene, err := engine.ParseScene(strings.NewReader(text))
	if err != nil {
		var in *engine.InputError
		if errors.As(err, &in) {
			return reply(map[string]string{"error": in.Msg})
		}
		return reply(map[string]string{"error": "the scene could not be read"})
	}
	res, err := engine.Run(scene)
	if err != nil {
		return reply(map[string]string{"error": err.Error()})
	}
	return reply(res)
}

func catalogue(_ js.Value, _ []js.Value) any { return reply(engine.Catalogue) }

func main() {
	js.Global().Set("shadegrove", js.ValueOf(map[string]any{
		"plan":      js.FuncOf(plan),
		"catalogue": js.FuncOf(catalogue),
		"ready":     true,
	}))
	select {} // keep the functions alive
}
