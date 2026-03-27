/*
The MIT License (MIT)

Copyright (c) 2022-2023 Reliza Incorporated (Reliza (tm), https://reliza.io)

Permission is hereby granted, free of charge, to any person obtaining a copy of this software and associated documentation files (the "Software"),
to deal in the Software without restriction, including without limitation the rights to use, copy, modify, merge, publish, distribute, sublicense,
and/or sell copies of the Software, and to permit persons to whom the Software is furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*/
package cli

import (
	"testing"
)

func TestParseWatcherNamespacesFromJson(t *testing.T) {
	testJson := `{
		"apiVersion": "apps/v1",
		"kind": "Deployment",
		"metadata": {
			"name": "test-watcher-deployment",
			"namespace": "test-ns"
		},
		"spec": {
			"template": {
				"spec": {
					"containers": [
						{
							"name": "test-watcher",
							"image": "registry.example.com/test/watcher-app:1.2.3@sha256:aabbccdd",
							"env": [
								{
									"name": "REARM_URI",
									"value": "https://test.rearmhq.example.com"
								},
								{
									"name": "NAMESPACE",
									"value": "alpha-ns,beta-ns,gamma-ns"
								},
								{
									"name": "SENDER_ID",
									"value": "default"
								},
								{
									"name": "REARM_API_ID",
									"valueFrom": {
										"secretKeyRef": {
											"key": "rearm-api-id",
											"name": "rearm-watcher"
										}
									}
								},
								{
									"name": "REARM_API_KEY",
									"valueFrom": {
										"secretKeyRef": {
											"key": "rearm-api-key",
											"name": "rearm-watcher"
										}
									}
								}
							]
						}
					]
				}
			}
		}
	}`
	expected := "alpha-ns,beta-ns,gamma-ns"
	actual := parseWatcherNamespacesFromJson(testJson)
	if expected != actual {
		t.Fatalf("actual namespaces = %q, expected = %q", actual, expected)
	}
}

func TestParseWatcherNamespacesFromJson_NoNamespaceEnv(t *testing.T) {
	testJson := `{
		"apiVersion": "apps/v1",
		"kind": "Deployment",
		"metadata": {
			"name": "test-watcher-deployment",
			"namespace": "test-ns"
		},
		"spec": {
			"template": {
				"spec": {
					"containers": [
						{
							"name": "test-watcher",
							"env": [
								{
									"name": "REARM_URI",
									"value": "https://test.rearmhq.example.com"
								}
							]
						}
					]
				}
			}
		}
	}`
	actual := parseWatcherNamespacesFromJson(testJson)
	if actual != "" {
		t.Fatalf("expected empty string when NAMESPACE env not present, got %q", actual)
	}
}

func TestWatcherNamespaceAggregation(t *testing.T) {
	namespacesForWathcer := make(map[string]bool)
	namespacesForWathcer["default"] = true
	namespacesForWathcer["myns1"] = true
	nsForWatcherStr := constructNamespaceStringFromMap(&namespacesForWathcer)
	expectedStr := "default\\,myns1"
	if expectedStr != nsForWatcherStr {
		t.Fatalf("actual nsForWatcherStr = %s , expected = %s", nsForWatcherStr, expectedStr)
	}
}
