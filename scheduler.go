package main

import (
	"context"
	"github.com/yuin/gopher-lua"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v60/github"
)

func runScheduledJobs() {
	for {
		log.Println("[scheduler] Running scheduled rules...")
		pk, err := os.ReadFile(privateKeyPath)
		if err != nil {
			log.Println("Failed to read private key:", err)
			time.Sleep(24 * time.Hour)
			continue
		}

		appTransport, err := ghinstallation.NewAppsTransport(http.DefaultTransport, appID, pk)
		if err != nil {
			log.Println("App transport error:", err)
			time.Sleep(24 * time.Hour)
			continue
		}

		client := github.NewClient(&http.Client{Transport: appTransport})
		installs, _, err := client.Apps.ListInstallations(context.Background(), nil)
		if err != nil {
			log.Println("Failed to list installations:", err)
			time.Sleep(24 * time.Hour)
			continue
		}

		for _, inst := range installs {
			go processScheduledForInstallation(inst, pk)
		}

		time.Sleep(24 * time.Hour)
	}
}

func processScheduledForInstallation(inst *github.Installation, pk []byte) {
	itr, err := ghinstallation.New(http.DefaultTransport, appID, inst.GetID(), pk)
	if err != nil {
		log.Printf("[scheduler] Install transport error for %s: %v", inst.Account.GetLogin(), err)
		return
	}
	client := github.NewClient(&http.Client{Transport: itr})

	repos, _, err := client.Apps.ListRepos(context.Background(), &github.ListOptions{PerPage: 100})
	if err != nil {
		log.Printf("[scheduler] List repos error for %s: %v", inst.Account.GetLogin(), err)
		return
	}

	for _, repo := range repos.Repositories {
		rules := fetchRulesFromRepo(repo.Owner.GetLogin(), repo.GetName())
		for _, rule := range rules {
			if rule.Event != "schedule" {
				continue
			}
			L := lua.NewState()
			exposeAPIFunctions(L)
			L.SetGlobal("event", &lua.LTable{})

			if rule.If != "" {
				if err := L.DoString("if not (" + rule.If + ") then return end"); err != nil {
					log.Printf("[scheduled %s/%s] if failed: %v", repo.GetFullName(), rule.Name, err)
					continue
				}
			}

			if err := L.DoString(rule.Run); err != nil {
				log.Printf("[scheduled %s/%s] run failed: %v", repo.GetFullName(), rule.Name, err)
			}
		}
	}
}
