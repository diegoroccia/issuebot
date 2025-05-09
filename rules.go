package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v60/github"
	"gopkg.in/yaml.v3"
)

type Rule struct {
	Name  string `yaml:"name"`
	Event string `yaml:"event"`
	If    string `yaml:"if"`
	Run   string `yaml:"run"`
}

type RuleFile struct {
	Rules []Rule `yaml:"rules"`
}

func fetchRulesFromRepo(owner, repo string) []Rule {
	pk, err := os.ReadFile(privateKeyPath)
	if err != nil {
		log.Println("Failed to read private key:", err)
		return nil
	}

	itr, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, pk)
	if err != nil {
		log.Println("Failed to create app transport:", err)
		return nil
	}

	client := github.NewClient(&http.Client{Transport: itr})
	installs, _, err := client.Apps.ListInstallations(context.Background(), nil)
	if err != nil {
		log.Println("Error listing installations:", err)
		return nil
	}

	for _, inst := range installs {
		if inst.Account.GetLogin() == owner {
			installationID = inst.GetID()
			break
		}
	}

	if installationID == 0 {
		log.Println("Installation not found for", owner)
		return nil
	}

	itrInstall, err := ghinstallation.New(http.DefaultTransport, appID, installationID, pk)
	if err != nil {
		log.Println("Failed to create installation transport:", err)
		return nil
	}
	client = github.NewClient(&http.Client{Transport: itrInstall})

	ctx := context.Background()

	// TODO: make the branch configurable - or check the default branch
	opt := &github.RepositoryContentGetOptions{Ref: "main"}

	_, files, _, err := client.Repositories.GetContents(ctx, owner, repo, ".github/issuebot", opt)
	if err != nil {
		log.Println("Failed to get rules directory:", err)
		return nil
	}

	var allRules []Rule
	for _, f := range files {
		if strings.HasSuffix(f.GetName(), ".yaml") {
			rc, _, err := client.Repositories.DownloadContents(ctx, owner, repo, f.GetPath(), opt)
			if err != nil {
				log.Printf("Error downloading %s: %v", f.GetName(), err)
				continue
			}
			buf, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				log.Printf("Error reading content %s: %v", f.GetName(), err)
				continue
			}
			var rf RuleFile
			err = yaml.Unmarshal(buf, &rf)
			if err != nil {
				log.Printf("YAML parse error in %s: %v", f.GetName(), err)
				continue
			}
			allRules = append(allRules, rf.Rules...)
		}
	}
	return allRules
}
