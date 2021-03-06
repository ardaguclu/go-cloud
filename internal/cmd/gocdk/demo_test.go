// Copyright 2019 The Go Cloud Development Kit Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAddDemo(t *testing.T) {
	ctx := context.Background()
	pctx, cleanup, err := newTestProject(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Call the main package run function as if 'gocdk demo add' were being called
	// from the command line for each of the demos.
	for _, demo := range allDemos {
		if err := run(ctx, pctx, []string{"demo", "add", demo}); err != nil {
			t.Fatalf("run demo error: %+v", err)
		}
	}

	// Build the binary.
	exePath := filepath.Join(pctx.workdir, "add-demo-test")
	if runtime.GOOS == "windows" {
		exePath += ".EXE"
	}
	if err := buildForServe(ctx, pctx, pctx.workdir, exePath); err != nil {
		t.Fatal("buildForServe(...):", err)
	}

	// Update the environment to use local implementations for each portable API.
	pctx.env = overrideEnv(pctx.env,
		"BLOB_BUCKET_URL=mem://",
		"DOCSTORE_COLLECTION_URL=mem://mycollection/Key",
		"PUBSUB_TOPIC_URL=mem://testtopic",
		"PUBSUB_SUBSCRIPTION_URL=mem://testtopic",
		"RUNTIMEVAR_VARIABLE_URL=constant://?val=test-variable-value&decoder=string",
		"SECRETS_KEEPER_URL=base64key://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=",
	)

	// Run the program, listening on a free port.
	alloc := &serverAlloc{exePath: exePath, port: findFreePort()}
	cmd, err := alloc.start(ctx, pctx, pctx.errlog, pctx.workdir, nil)
	if err != nil {
		t.Fatalf("failed to start server: %v", err)
	}
	defer endServerProcess(cmd)

	tests := []struct {
		demo          string
		description   string
		urlPaths      []string
		op            string
		urlQuery      string
		urlValues     url.Values // only used if op=POST
		stringsToFind []string
	}{
		// BLOB TESTS.
		{
			demo:        "blob",
			description: "base",
			urlPaths:    []string{"/demo/blob", "/demo/blob/"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"This page demonstrates the use",
				"https://gocloud.dev/howto/blob",
				`<a href="./list">List</a>`,
				`<a href="./write">Write</a>`,
			},
		},
		{
			demo:        "blob",
			description: "list: empty bucket",
			urlPaths:    []string{"/demo/blob/list"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"no blobs in bucket",
			},
		},
		{
			demo:        "blob",
			description: "write: empty form",
			urlPaths:    []string{"/demo/blob/write"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			demo:        "blob",
			description: "write: missing key",
			urlPaths:    []string{"/demo/blob/write"},
			op:          "POST",
			urlValues:   map[string][]string{"contents": {"foo"}},
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"<strong>enter a non-empty key to write to</strong>",
				"foo", // previous entry for contents field is carried over
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			demo:        "blob",
			description: "write: missing contents",
			urlPaths:    []string{"/demo/blob/write"},
			op:          "POST",
			urlValues:   map[string][]string{"key": {"key1"}},
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"<strong>enter some content to write</strong>",
				"key1", // previous entry for key field is carried over
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			demo:        "blob",
			description: "write: top level key",
			urlPaths:    []string{"/demo/blob/write"},
			op:          "POST",
			urlValues:   map[string][]string{"key": {"key1"}, "contents": {"key1 contents"}},
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"Wrote it!",
			},
		},
		{
			demo:        "blob",
			description: "write: subdirectory key",
			urlPaths:    []string{"/demo/blob/write"},
			op:          "POST",
			urlValues:   map[string][]string{"key": {"subdir/key2"}, "contents": {"key2 contents"}},
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				"Wrote it!",
			},
		},
		{
			demo:        "blob",
			description: "list: no longer empty",
			urlPaths:    []string{"/demo/blob/list"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				`<a href="./view?key=key1">key1</a>`,
				`<a href="./list?prefix=subdir%2f">subdir/</a>`,
			},
		},
		{
			demo:        "blob",
			description: "list: subdir",
			urlPaths:    []string{"/demo/blob/list"},
			urlQuery:    "prefix=subdir%2f",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/blob demo</title>",
				`<a href="./view?key=subdir%2fkey2">subdir/key2</a>`,
			},
		},
		{
			demo:        "blob",
			description: "view: key1 selected",
			urlPaths:    []string{"/demo/blob/view"},
			urlQuery:    "key=key1",
			op:          "GET",
			stringsToFind: []string{
				"key1 contents",
			},
		},
		{
			demo:        "blob",
			description: "view: key2 selected",
			urlPaths:    []string{"/demo/blob/view"},
			urlQuery:    "key=subdir%2fkey2",
			op:          "GET",
			stringsToFind: []string{
				"key2 contents",
			},
		},
		// DOCSTORE TESTS.
		{
			demo:        "docstore",
			description: "base",
			urlPaths:    []string{"/demo/docstore", "/demo/docstore/"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"This page demonstrates the use",
				"https://gocloud.dev/howto/docstore",
				`<a href="./list">List</a>`,
				`<a href="./edit?create=true">Create</a>`,
			},
		},
		{
			demo:        "docstore",
			description: "list: empty collection",
			urlPaths:    []string{"/demo/docstore/list"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"no documents in collection",
			},
		},
		{
			demo:        "docstore",
			description: "create: empty form",
			urlPaths:    []string{"/demo/docstore/edit"},
			urlQuery:    "create=true",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			demo:        "docstore",
			description: "create: empty key",
			urlPaths:    []string{"/demo/docstore/edit"},
			op:          "POST",
			urlValues: map[string][]string{
				"create": {"true"},
				"value":  {"foo"},
			},
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"<strong>enter a non-empty key</strong>",
				"foo",                                     // previous entry for value field is carried over
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			demo:        "docstore",
			description: "create: success",
			urlPaths:    []string{"/demo/docstore/edit"},
			op:          "POST",
			urlValues: map[string][]string{
				"create": {"true"},
				"key":    {"key1"},
				"value":  {"initial value"},
			},
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"Write succeeded!",
			},
		},
		{
			demo:        "docstore",
			description: "list: no longer empty",
			urlPaths:    []string{"/demo/docstore/list"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				`<a href="./edit?key=key1">key1</a>`,
			},
		},
		{
			demo:        "docstore",
			description: "edit: initial form",
			urlPaths:    []string{"/demo/docstore/edit"},
			urlQuery:    "key=key1",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"key1",
				"initial value", // value is shown
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		{
			demo:        "docstore",
			description: "edit: initial form missing key",
			urlPaths:    []string{"/demo/docstore/edit"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"<strong>key must be provided to edit</strong>",
			},
		},
		{
			demo:        "docstore",
			description: "edit: initial form invalid key",
			urlPaths:    []string{"/demo/docstore/edit"},
			urlQuery:    "key=invalid",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"<strong>failed to get document",
			},
		},
		{
			demo:        "docstore",
			description: "edit: success",
			urlPaths:    []string{"/demo/docstore/edit"},
			op:          "POST",
			urlValues: map[string][]string{
				"key":   {"key1"},
				"value": {"updated value"},
			},
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"Write succeeded!",
			},
		},
		{
			demo:        "docstore",
			description: "edit: showing updated value",
			urlPaths:    []string{"/demo/docstore/edit"},
			urlQuery:    "key=key1",
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/docstore demo</title>",
				"key1",
				"updated value", // updated value is shown
				`<input type="submit" value="Write It!">`, // form is shown
			},
		},
		// PUBSUB TESTS.
		{
			demo:        "pubsub",
			description: "base",
			urlPaths:    []string{"/demo/pubsub", "/demo/pubsub/"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/pubsub demo</title>",
				"This page demonstrates the use",
				"https://gocloud.dev/howto/pubsub",
				`<a href="./send">Send</a>`,
				`<a href="./receive">Receive</a>`,
				"Enter a message to send to the topic",
			},
		},
		{
			demo:        "pubsub",
			description: "empty receive",
			urlPaths:    []string{"/demo/pubsub/receive"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/pubsub demo</title>",
				"This page demonstrates the use",
				"https://gocloud.dev/howto/pubsub",
				`<a href="./send">Send</a>`,
				`<a href="./receive">Receive</a>`,
				"No message available",
			},
		},
		{
			demo:        "pubsub",
			description: "send",
			urlPaths:    []string{"/demo/pubsub/", "/demo/pubsub/send"},
			op:          "POST",
			urlValues:   map[string][]string{"msg": {"hello world"}},
			stringsToFind: []string{
				"<title>gocloud.dev/pubsub demo</title>",
				"This page demonstrates the use",
				"https://gocloud.dev/howto/pubsub",
				`<a href="./send">Send</a>`,
				`<a href="./receive">Receive</a>`,
				"Message sent!",
			},
		},
		{
			demo:        "pubsub",
			description: "receive",
			urlPaths:    []string{"/demo/pubsub/receive"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/pubsub demo</title>",
				"This page demonstrates the use",
				"https://gocloud.dev/howto/pubsub",
				`<a href="./send">Send</a>`,
				`<a href="./receive">Receive</a>`,
				"Received message:",
				"hello world",
			},
		},
		// RUNTIMEVAR TESTS.
		{
			demo:        "runtimevar",
			description: "base",
			urlPaths:    []string{"/demo/runtimevar", "/demo/runtimevar/"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/runtimevar demo</title>",
				"This page demonstrates the use",
				"https://gocloud.dev/howto/runtimevar",
				"The current value of the variable",
				"test-variable-value",
				"It was last modified",
			},
		},
		// SECRETS TESTS.
		{
			demo:        "secrets",
			description: "base page shows encrypt form",
			urlPaths:    []string{"/demo/secrets", "/demo/secrets/", "/demo/secrets/encrypt"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/secrets demo</title>",
				"This page demonstrates the use",
				"https://gocloud.dev/howto/secrets",
				`<a href="./encrypt">Encrypt</a>`,
				`<a href="./decrypt">Decrypt</a>`,
				"Enter plaintext data to encrypt",
			},
		},
		{
			demo:        "secrets",
			description: "encrypt works",
			urlPaths:    []string{"/demo/secrets/", "/demo/secrets/encrypt"},
			op:          "POST",
			urlValues: map[string][]string{
				"plaintext": {"my-sample-plaintext"},
			},
			stringsToFind: []string{
				"<title>gocloud.dev/secrets demo</title>",
				`<a href="./encrypt">Encrypt</a>`,
				`<a href="./decrypt">Decrypt</a>`,
				"Enter plaintext data to encrypt",
				"my-sample-plaintext", // input carries over
				"Encrypted result",
				"Decrypt it", // form button to decrypt it
			},
		},
		{
			demo:        "secrets",
			description: "encrypt fails on invalid base64 input",
			urlPaths:    []string{"/demo/secrets/", "/demo/secrets/encrypt"},
			op:          "POST",
			urlValues: map[string][]string{
				"plaintext": {"this-is-not-base64"},
				"base64":    {"true"},
			},
			stringsToFind: []string{
				"<title>gocloud.dev/secrets demo</title>",
				`<a href="./encrypt">Encrypt</a>`,
				`<a href="./decrypt">Decrypt</a>`,
				"Enter plaintext data to encrypt",
				"this-is-not-base64", // input carries over
				"checked",            // base64 checkbox stays checked
				"Plaintext data was not valid Base64",
			},
		},
		{
			demo:        "secrets",
			description: "decrypt empty form",
			urlPaths:    []string{"/demo/secrets/decrypt"},
			op:          "GET",
			stringsToFind: []string{
				"<title>gocloud.dev/secrets demo</title>",
				`<a href="./encrypt">Encrypt</a>`,
				`<a href="./decrypt">Decrypt</a>`,
				"Enter base64-encoded data to decrypt",
			},
		},
		{
			demo:        "secrets",
			description: "decrypt works",
			urlPaths:    []string{"/demo/secrets/decrypt"},
			op:          "POST",
			urlValues: map[string][]string{
				// "hello world" encrypted, then base64-encoded.
				"ciphertext": {"6AzIcsNNnPv0x7jkVnNRoHI2mf4UY94N0pInrQM8AhFxoU7ZKeiPLaF5YHpTDBmRe8rw"},
			},
			stringsToFind: []string{
				"<title>gocloud.dev/secrets demo</title>",
				`<a href="./encrypt">Encrypt</a>`,
				`<a href="./decrypt">Decrypt</a>`,
				"Enter base64-encoded data to decrypt",
				"6AzIcsNNnPv0x7jkVnNRoHI2mf4UY94N0pInrQM8AhFxoU7ZKeiPLaF5YHpTDBmRe8rw", // input carries over
				"Decrypted result",
				"hello world",
				"Encrypt it", // form button to re-encrypt it
			},
		},
		{
			demo:        "secrets",
			description: "decrypt works with base64 output",
			urlPaths:    []string{"/demo/secrets/decrypt"},
			op:          "POST",
			urlValues: map[string][]string{
				"base64": {"true"},
				// "hello world" encrypted, then base64-encoded.
				"ciphertext": {"6AzIcsNNnPv0x7jkVnNRoHI2mf4UY94N0pInrQM8AhFxoU7ZKeiPLaF5YHpTDBmRe8rw"},
			},
			stringsToFind: []string{
				"<title>gocloud.dev/secrets demo</title>",
				`<a href="./encrypt">Encrypt</a>`,
				`<a href="./decrypt">Decrypt</a>`,
				"Enter base64-encoded data to decrypt",
				"6AzIcsNNnPv0x7jkVnNRoHI2mf4UY94N0pInrQM8AhFxoU7ZKeiPLaF5YHpTDBmRe8rw", // input carries over
				"checked", // base64 checkbox stays checked
				"Decrypted result",
				"aGVsbG8gd29ybGQ=", // "hello world" base64 encoded
				"Encrypt it",       // form button to re-encrypt it
			},
		},
		{
			demo:        "secrets",
			description: "decrypt fails on invalid base64 input",
			urlPaths:    []string{"/demo/secrets/decrypt"},
			op:          "POST",
			urlValues: map[string][]string{
				"ciphertext": {"this-is-not-base64"},
			},
			stringsToFind: []string{
				"<title>gocloud.dev/secrets demo</title>",
				`<a href="./encrypt">Encrypt</a>`,
				`<a href="./decrypt">Decrypt</a>`,
				"Enter base64-encoded data to decrypt",
				"this-is-not-base64", // input carries over
			},
		},
	}

	for _, test := range tests {
		for _, urlPath := range test.urlPaths {
			queryURL := alloc.url(urlPath)
			queryURL.RawQuery = test.urlQuery
			u := queryURL.String()
			t.Run(fmt.Sprintf("%s %s: %s", test.demo, test.description, u), func(t *testing.T) {
				var resp *http.Response
				var err error
				switch test.op {
				case "GET":
					resp, err = http.DefaultClient.Get(u)
				case "POST":
					resp, err = http.DefaultClient.PostForm(u, test.urlValues)
				default:
					t.Fatalf("invalid test.op: %q", test.op)
				}
				if err != nil {
					t.Fatalf("HTTP %q request failed: %v", test.op, err)
				}
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					t.Fatalf("HTTP request returned status code %v, want %v", resp.StatusCode, http.StatusOK)
				}
				bodyBytes, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("failed to read HTTP response body: %v", err)
				}
				body := string(bodyBytes)
				logBody := false // only log the body on failure, and only once per test
				for _, s := range test.stringsToFind {
					if !strings.Contains(body, s) {
						t.Errorf("didn't find %q in HTTP response body", s)
						logBody = true
					}
				}
				if logBody {
					t.Error("Full HTTP response body:\n\n", body)
				}
			})
		}
	}
}

func findFreePort() int {
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatalf("failed to Listen to localhost:0; no free ports?: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
