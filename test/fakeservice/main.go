// This file creates a dummy webserver with the sole pupose of being used
// as a binary to test the services.Service struct
package main

import "net/http"

func main() {
	err := http.ListenAndServe(":8090", nil)
	panic(err)
}
