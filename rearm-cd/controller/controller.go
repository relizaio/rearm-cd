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
package controller

import (
	"os"
	"strings"
	"time"

	"github.com/relizaio/rearm-cd/cli"
	"github.com/relizaio/rearm-cd/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var sugar *zap.SugaredLogger

func init() {
	config := zap.NewProductionConfig()
	config.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout(time.RFC3339)
	if strings.ToLower(os.Getenv("LOG_LEVEL")) == "debug" {
		config.Level.SetLevel(zap.DebugLevel)
	}
	var logger, _ = config.Build()
	defer logger.Sync()
	sugar = logger.Sugar()
}

func loopInit() {
	sugar.Info("Starting loopInit - getting sealed cert")
	sealedCert := cli.GetSealedCert()
	sugar.Info("Got sealed cert, length: ", len(sealedCert))
	if len(sealedCert) < 1 {
		sugar.Info("Sealed cert is empty, installing sealed certificates")
		cli.InstallSealedCertificates()
		for len(sealedCert) < 1 {
			sealedCert = cli.GetSealedCert()
			time.Sleep(3 * time.Second)
		}
		sugar.Info("Installed Bitnami Sealed Certificates")
	}

	sugar.Info("Setting sealed certificate on the hub")
	cli.SetSealedCertificateOnTheHub(sealedCert)
	sugar.Info("Completed loopInit")
}

func singleLoopRun() {
	instManifest, err := cli.GetInstanceCycloneDX()

	if err != nil {
		sugar.Error(err)
	}

	if err == nil {
		rlzDeployments := cli.ParseInstanceCycloneDXIntoDeployments(instManifest)

		existingDeployments := collectExistingDeployments()

		namespacesForWatcher := make(map[string]bool)

		isError := false
		didDeploy := false

		for _, rd := range rlzDeployments {
			if rd.IntegrationType == "NONE" || (rd.IntegrationType == "TARGET" && rd.ArtVersion == "") {
				// Mark as seen to prevent uninstall, but skip install/upgrade
				// Do NOT add namespace to watcher - if all products in this namespace are NONE, it shouldn't be monitored
				existingDeployments[rd.Name] = true
				sugar.Debugw("Skipping deployment due to integrationType=NONE or TARGET without version",
					"product", rd.Product,
					"deploymentName", rd.Name)
				continue
			}
			if rd.IntegrationType == "UNINSTALL" {
				// Do NOT mark as seen - leave existingDeployments[rd.Name] = false
				// so deleteObsoleteDeployments will uninstall it if present
				namespacesForWatcher[rd.Namespace] = true
				sugar.Debugw("Marking deployment for uninstall due to integrationType=UNINSTALL",
					"product", rd.Product,
					"deploymentName", rd.Name)
				continue
			}
			existingDeployments[rd.Name] = true
			deployed, err := processSingleDeployment(&rd)
			if err != nil {
				// Errors already logged in processSingleDeployment with full context
				sugar.Infow("Skipping deployment due to error",
					"product", rd.Product,
					"version", rd.ArtVersion,
					"namespace", rd.Namespace,
					"deploymentName", rd.Name)
			}
			isError = (err != nil)
			if deployed {
				didDeploy = true
			}
			namespacesForWatcher[rd.Namespace] = true
		}

		cli.InstallWatcher(&namespacesForWatcher)

		if !isError && len(rlzDeployments) > 0 {
			deleteObsoleteDeployments(&existingDeployments)
		}

		if didDeploy {
			helmDataStreamToHub(&existingDeployments)
		}
	}
}

func Loop() {
	loopInit()

	go cli.StartBackupScheduler()

	for true {
		singleLoopRun()
		time.Sleep(15 * time.Second)
	}
}

func helmDataStreamToHub(existingDeployments *map[string]bool) {
	// collect per namespace
	perNamespaceActiveDepl := map[string]cli.PathsPerNamespace{}
	for edKey, edVal := range *existingDeployments {
		ns := getNamespaceFromPath(edKey)
		curPaths, exists := perNamespaceActiveDepl[ns]
		if exists && edVal {
			curPaths.Paths = append(curPaths.Paths, "workspace/"+edKey+"/")
			perNamespaceActiveDepl[ns] = curPaths
		} else if edVal {
			curPaths = cli.PathsPerNamespace{}
			curPaths.Paths = append(curPaths.Paths, "workspace/"+edKey+"/")
			curPaths.Namespace = ns
			perNamespaceActiveDepl[ns] = curPaths
		} else if !exists {
			curPaths = cli.PathsPerNamespace{}
			curPaths.Paths = []string{}
			curPaths.Namespace = ns
			perNamespaceActiveDepl[ns] = curPaths
		}
	}

	for _, ppn := range perNamespaceActiveDepl {
		cli.StreamHelmChartMetadataToHub(&ppn)
	}

}

func getNamespaceFromPath(path string) string {
	return strings.Split(path, "---")[0]
}

func deleteObsoleteDeployments(existingDeployments *map[string]bool) {
	for edKey, edVal := range *existingDeployments {
		if !edVal {
			cli.DeleteObsoleteDeployment("workspace/" + edKey + "/")
		}
	}
}

func collectExistingDeployments() map[string]bool {
	existingDeployments := make(map[string]bool)
	workspaceEntries, err := os.ReadDir("workspace")
	if err != nil {
		sugar.Error(err)
	}
	for _, we := range workspaceEntries {
		if we.IsDir() && we.Name() != "watcher" && we.Name() != "lost+found" {
			existingDeployments[we.Name()] = false
		}
	}
	return existingDeployments
}

func processSingleDeployment(rd *cli.RearmDeployment) (bool, error) {
	if cli.SecretsNamespace == "" {
		sugar.Info("SecretNS is null")
		panic("secretnamespace must be set by this point")
	}
	var compAuth cli.ComponentAuth
	if rd.ArtHash.Value == "" {
		// No hash means public repo, assume NOCREDS
		compAuth.Type = "NOCREDS"
	} else {
		digest := cli.ExtractRlzDigestFromCdxDigest(rd.ArtHash)
		compAuth = cli.GetComponentAuthByDeliverableDigest(digest, rd.Namespace)
	}
	dirName := rd.Name
	os.MkdirAll("workspace/"+dirName, 0700)
	groupPath := "workspace/" + dirName + "/"

	var helmDownloadPa cli.ComponentAuth

	doInstall := false
	isError := false
	helmDownloadPa.Type = compAuth.Type
	helmInfo := cli.GetHelmRepoInfoFromDeployment(rd)
	if compAuth.Type == "ECR" {
		ecrSecretPath := "workspace/" + dirName + "/ecrreposecret.yaml"
		ecrSecretFile := utils.CreateFile(ecrSecretPath)
		cli.ProduceEcrSecretYaml(ecrSecretFile, rd, compAuth, cli.SecretsNamespace)
		cli.KubectlApply(ecrSecretPath)
		cli.WaitUntilSecretCreated("ecr-"+rd.Name, cli.SecretsNamespace)
		ecrAuthPa := cli.ResolveHelmAuthSecret("ecr-" + dirName)
		ecrToken := getEcrToken(&ecrAuthPa)
		var paForPlainSecret cli.ComponentAuth
		paForPlainSecret.Login = "AWS"
		paForPlainSecret.Password = ecrToken
		paForPlainSecret.Type = "ECR"
		paForPlainSecret.Url = ecrAuthPa.Url
		secretPath := "workspace/" + dirName + "/reposecret.yaml"
		secretFile := utils.CreateFile(secretPath)
		cli.ProducePlainSecretYaml(secretFile, rd, paForPlainSecret, cli.SecretsNamespace, helmInfo)
		cli.KubectlApply(secretPath)
		cli.WaitUntilSecretCreated(rd.Name, cli.SecretsNamespace)
		helmDownloadPa = cli.ResolveHelmAuthSecret(dirName)
	}

	if compAuth.Type == "CREDS" {
		sugar.Debug("CREDS auth type detected for deployment: ", rd.Name, ", namespace: ", rd.Namespace)
		secretPath := "workspace/" + dirName + "/reposecret.yaml"
		sugar.Debug("Creating sealed secret at: ", secretPath)
		secretFile := utils.CreateFile(secretPath)
		cli.ProduceSecretYaml(secretFile, rd, compAuth, cli.SecretsNamespace, helmInfo)
		sugar.Debug("Applying sealed secret for: ", rd.Name)
		cli.KubectlApply(secretPath)
		sugar.Debug("Waiting for secret to be created: ", rd.Name, " in namespace: ", cli.SecretsNamespace)
		cli.WaitUntilSecretCreated(rd.Name, cli.SecretsNamespace)
		sugar.Debug("Resolving helm auth secret for: ", dirName)
		helmDownloadPa = cli.ResolveHelmAuthSecret(dirName)
		sugar.Debug("Helm download auth resolved, type: ", helmDownloadPa.Type)
	}

	if compAuth.Type == "NOCREDS" {
		secretPath := "workspace/" + dirName + "/reposecret.yaml"
		secretFile := utils.CreateFile(secretPath)
		cli.ProduceSecretYaml(secretFile, rd, compAuth, cli.SecretsNamespace, helmInfo)
		cli.KubectlApply(secretPath)
		cli.WaitUntilSecretCreated(rd.Name, cli.SecretsNamespace)
		helmDownloadPa.Url = rd.ArtUri
	}
	var err error
	lastHelmVer := cli.GetLastHelmVersion(groupPath)
	doDownloadChart := false
	if rd.ArtVersion != lastHelmVer {
		doDownloadChart = true
	} else {
		if _, err := os.Stat(groupPath + cli.GetChartNameFromDeployment(rd) + "/Chart.yaml"); err != nil {
			doDownloadChart = true
		}
	}
	if doDownloadChart {
		err = cli.DownloadHelmChart(groupPath, rd, &helmDownloadPa, helmInfo)
		if err == nil {
			cli.RecordHelmChartVersion(groupPath, rd)
			doInstall = true
		} else {
			// Error already logged in DownloadHelmChart with full context
			isError = true
		}
	}

	if !isError {
		err = cli.ResolvePreviousDiffFile(groupPath)
		isError = (err != nil)
	}

	if !isError {
		err = cli.MergeHelmValues(groupPath, rd)
		isError = (err != nil)
	}

	if !isError {
		err = cli.ReplaceTagsForDiff(groupPath, rd.Namespace)
		isError = (err != nil)
	}

	if !isError && !doInstall {
		doInstall = cli.IsValuesDiff(groupPath)
	}
	if !isError && !doInstall {
		doInstall = !cli.IsFirstInstallDone(rd)
	}

	if !isError && doInstall {
		err = cli.SetHelmChartAppVersion(groupPath, rd)
		isError = (err != nil)
	}

	if !isError && doInstall {
		err = cli.ReplaceTagsForInstall(groupPath, rd.Namespace)
		isError = (err != nil)
	}

	if !isError && doInstall {
		err := cli.InstallApplication(groupPath, rd)
		isError = (err != nil)
	}

	if !isError && doInstall {
		cli.RecordDeployedData(groupPath, rd)
	}

	return !isError && doInstall, err
}
