//go:build js && wasm

package main

import "syscall/js"

// request performs an HTTP call with fetch() and invokes cb with the status
// and body text. A status of 0 means the request itself failed.
func request(method, url string, body []byte, cb func(status int, data []byte)) {
	opts := js.Global().Get("Object").New()
	opts.Set("method", method)
	if body != nil {
		h := js.Global().Get("Object").New()
		h.Set("Content-Type", "application/json")
		opts.Set("headers", h)
		opts.Set("body", string(body))
	}
	var onResp, onText, onErr js.Func
	status := 0
	release := func() {
		onResp.Release()
		onText.Release()
		onErr.Release()
	}
	onResp = js.FuncOf(func(this js.Value, args []js.Value) any {
		status = args[0].Get("status").Int()
		return args[0].Call("text")
	})
	onText = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer release()
		cb(status, []byte(args[0].String()))
		return nil
	})
	onErr = js.FuncOf(func(this js.Value, args []js.Value) any {
		defer release()
		cb(0, nil)
		return nil
	})
	js.Global().Call("fetch", url, opts).Call("then", onResp).Call("then", onText).Call("catch", onErr)
}
