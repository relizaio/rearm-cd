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
	"errors"
	"testing"
)

func withClusterNamespaces(t *testing.T, names []string, err error) {
	t.Helper()
	orig := listClusterNamespaces
	listClusterNamespaces = func() (map[string]bool, error) {
		if err != nil {
			return nil, err
		}
		m := make(map[string]bool)
		for _, n := range names {
			m[n] = true
		}
		return m, nil
	}
	t.Cleanup(func() { listClusterNamespaces = orig })
}

// A namespace whose deployment has not created it yet must not reach the watcher: its chart creates a Role
// there, and the upgrade fails on every loop while that deployment keeps failing.
func TestWatcherSkipsNamespacesThatDoNotExist(t *testing.T) {
	withClusterNamespaces(t, []string{"rearm-cd", "scully-rearm"}, nil)
	in := map[string]bool{"rearm-cd": true, "scully-rearm": true, "scully-reliza-dtrack5": true}
	got := *onlyExistingNamespaces(&in)
	if len(got) != 2 || !got["rearm-cd"] || !got["scully-rearm"] || got["scully-reliza-dtrack5"] {
		t.Fatalf("want only the existing namespaces, got %v", got)
	}
	if constructNamespaceStringFromMap(&got) != `rearm-cd\,scully-rearm` {
		t.Errorf("namespace string = %q", constructNamespaceStringFromMap(&got))
	}
	if len(in) != 3 {
		t.Errorf("the caller's set must not be modified, got %v", in)
	}
}

func TestWatcherKeepsEveryNamespaceWhenTheyCannotBeListed(t *testing.T) {
	withClusterNamespaces(t, nil, errors.New("kubectl: connection refused"))
	in := map[string]bool{"rearm-cd": true, "not-yet": true}
	got := *onlyExistingNamespaces(&in)
	if len(got) != 2 {
		t.Fatalf("a failed listing must not strip the watcher, got %v", got)
	}
}

func TestWatcherWithNoNamespacesAsksNothing(t *testing.T) {
	withClusterNamespaces(t, nil, errors.New("must not be called"))
	empty := map[string]bool{}
	if got := onlyExistingNamespaces(&empty); len(*got) != 0 {
		t.Fatalf("got %v", *got)
	}
}
