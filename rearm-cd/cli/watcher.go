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
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	watcherPath                 = "workspace/watcher/"
	watcherHelmLastKnownVersion = watcherPath + "lastKnownWatcherHelmVersion"
	watcherDeploymentName       = "rearm-watcher-deployment"
)

func InstallWatcher(namespacesForWatcher *map[string]bool) {
	namespacesForWatcherStr := ""
	if nil != *namespacesForWatcher && len(*namespacesForWatcher) > 0 {
		namespacesForWatcherStr = constructNamespaceStringFromMap(namespacesForWatcher)
	}

	isWatcherConfigUpdated := isWatcherConfigUpdated(namespacesForWatcherStr)

	if isWatcherConfigUpdated {
		sugar.Info("Watcher config was updated, proceeding with install")
		installWatcherRoutine(namespacesForWatcherStr)
		recordWatcherConfig()
	}

}

func recordWatcherConfig() {
	os.MkdirAll(watcherPath, 0700)
	recVersionFile, err := os.Create(watcherHelmLastKnownVersion)
	if err != nil {
		sugar.Error(err)
	}
	recVersionFile.Write([]byte(watcherHelmVersion))
	recVersionFile.Close()
}

type watcherDeploymentEnvSpec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type watcherDeploymentContainerSpec struct {
	Env []watcherDeploymentEnvSpec `json:"env"`
}

type watcherDeploymentSpec struct {
	Template struct {
		Spec struct {
			Containers []watcherDeploymentContainerSpec `json:"containers"`
		} `json:"spec"`
	} `json:"template"`
}

type watcherDeploymentJson struct {
	Spec watcherDeploymentSpec `json:"spec"`
}

func parseWatcherNamespacesFromJson(jsonStr string) string {
	var dep watcherDeploymentJson
	if err := json.Unmarshal([]byte(jsonStr), &dep); err != nil {
		sugar.Error("Failed to parse watcher deployment JSON: ", err)
		return ""
	}
	for _, container := range dep.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if env.Name == "NAMESPACE" {
				return env.Value
			}
		}
	}
	return ""
}

func GetWatcherCurrentNamespaces() string {
	out, _, err := shellout(KubectlApp + " get deployment " + watcherDeploymentName + " -n " + RearmCdNamespace + " -o json")
	if err != nil {
		sugar.Debug("Watcher deployment not found or not readable, treating as not installed")
		return ""
	}
	return parseWatcherNamespacesFromJson(out)
}

func isWatcherConfigUpdated(namespacesForWatcherStr string) bool {
	// Check live namespace config from the running watcher deployment
	// namespacesForWatcherStr uses \, as helm --set separator; live deployment stores plain ,
	// Sort both sides before comparing to avoid false positives from ordering differences
	currentNamespaces := GetWatcherCurrentNamespaces()
	desiredNamespaces := strings.ReplaceAll(namespacesForWatcherStr, `\,`, ",")
	if strings.Compare(sortNamespaceString(desiredNamespaces), sortNamespaceString(currentNamespaces)) != 0 {
		sugar.Infow("Watcher namespace config changed",
			"current", currentNamespaces,
			"desired", namespacesForWatcherStr)
		return true
	}

	// Check if helm chart version changed (tracked in file)
	prevVersion, err := os.ReadFile(watcherHelmLastKnownVersion)
	if err != nil && os.IsNotExist(err) {
		return true
	} else if err != nil {
		sugar.Error(err)
		return false
	} else if strings.Compare(watcherHelmVersion, string(prevVersion)) != 0 {
		return true
	}
	return false
}

func installWatcherRoutine(namespacesForWatcherStr string) {
	dryRunShellout(KubectlApp + " create secret generic rearm-watcher -n " + RearmCdNamespace + " --from-literal=rearm-api-id=" + os.Getenv("REARM_APIKEYID") + " --from-literal=rearm-api-key=" + os.Getenv("REARM_APIKEY") + " --dry-run=client -o yaml | " + KubectlApp + " apply -f -")
	rearmUri := os.Getenv("REARM_URI")
	if len(rearmUri) < 1 {
		sugar.Fatal("URI environment variable must be defined for watcher installation")
	}
	watcherImageSet := ""
	if len(watcherImage) > 0 {
		watcherImageSet = " --set image.repository=" + watcherImage
	}
	retryLeft := 3
	watcherInstalled := false
	for !watcherInstalled && retryLeft > 0 {
		_, _, err := dryRunShellout(HelmApp + " upgrade --install rearm-watcher -n " + RearmCdNamespace + " --set namespace=\"" + namespacesForWatcherStr + "\" --set rearmUri=" + rearmUri + watcherImageSet + " --version " + watcherHelmVersion + " " + watcherHelmChart)
		if err == nil {
			watcherInstalled = true
		} else {
			retryLeft--
			sugar.Warn("Could not install watcher, retries left = ", retryLeft)
			time.Sleep(2 * time.Second)
		}
	}
}

func sortNamespacesForWatcher(namespacesForWatcher *map[string]bool) []string {
	var sortedNamespaces []string
	for nskey := range *namespacesForWatcher {
		sortedNamespaces = append(sortedNamespaces, nskey)
	}
	if len(sortedNamespaces) > 1 {
		sort.Slice(sortedNamespaces, func(i, j int) bool {
			return sortedNamespaces[i] < sortedNamespaces[j]
		})
	}
	return sortedNamespaces
}

func sortNamespaceString(namespacesStr string) string {
	if namespacesStr == "" {
		return ""
	}
	parts := strings.Split(namespacesStr, ",")
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func constructNamespaceStringFromMap(namespacesForWatcher *map[string]bool) string {
	sortedNamespaces := sortNamespacesForWatcher(namespacesForWatcher)
	namespacenamespacesForWatcherStr := ""
	for _, nskey := range sortedNamespaces {
		namespacenamespacesForWatcherStr += nskey + "\\,"
	}
	re := regexp.MustCompile(`\\,$`)
	nsByteArr := re.ReplaceAll([]byte(namespacenamespacesForWatcherStr), []byte(""))
	namespacenamespacesForWatcherStr = string(nsByteArr)
	return namespacenamespacesForWatcherStr
}
