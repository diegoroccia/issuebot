package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v60/github"
	"github.com/yuin/gopher-lua"
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

var (
	appID          = int64(1252582)      // replace with your GitHub App ID
	installationID = int64(0)            // populated dynamically per webhook
	privateKeyPath = "./private-key.pem" // path to your GitHub App private key
)

func main() {
	go runScheduledJobs() // Start scheduler in background

	http.HandleFunc("/webhook", handleWebhook)
	log.Println("Listening on :3000")
	http.ListenAndServe(":80", nil)
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, []byte(os.Getenv("GITHUB_WEBHOOK_SECRET")))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	eventType := github.WebHookType(r)
	delivery := github.DeliveryID(r)
	log.Printf("[%s] Received event: %s", delivery, eventType)

	owner, repo := extractRepo(event)
	if owner == "" || repo == "" {
		log.Println("Could not determine repo owner/name from event")
		return
	}

	rules := fetchRulesFromRepo(owner, repo)

	for _, rule := range rules {
		if rule.Event != eventType {
			continue
		}
		L := lua.NewState()
		exposeAPIFunctions(L)
		exposeEvent(L, event)
		if rule.If != "" {
			err := L.DoString("if not (" + rule.If + ") then return end")
			if err != nil {
				log.Printf("[rule %s] if check failed: %v", rule.Name, err)
				continue
			}
		}
		err := L.DoString(rule.Run)
		if err != nil {
			log.Printf("[rule %s] run failed: %v", rule.Name, err)
		}
	}
}

func exposeAPIFunctions(L *lua.LState) {
	L.SetGlobal("assign", L.NewFunction(func(L *lua.LState) int {
		user := L.ToString(1)
		fmt.Println("Assigning to", user)
		return 0
	}))

	L.SetGlobal("label", L.NewFunction(func(L *lua.LState) int {
		label := L.ToString(1)
		fmt.Println("Adding label", label)
		return 0
	}))

	L.SetGlobal("close_issue", L.NewFunction(func(L *lua.LState) int {
		fmt.Println("Closing issue")
		return 0
	}))
}

func exposeEvent(L *lua.LState, event any) {
	tbl := L.NewTable()
	switch e := event.(type) {
	case *github.IssueCommentEvent:
		tbl.RawSetString("type", lua.LString("issue_comment"))
		if e.Comment != nil {
			tbl.RawSetString("comment", lua.LString(e.Comment.GetBody()))
		}
	case *github.ProjectCardEvent:
		tbl.RawSetString("type", lua.LString("project_card:moved"))
		if e.ProjectCard != nil {
			tbl.RawSetString("new_column", lua.LString(e.ProjectCard.GetColumnName()))
		}
	default:
		tbl.RawSetString("type", lua.LString("unknown"))
	}
	L.SetGlobal("event", tbl)
}

func extractRepo(event any) (owner, repo string) {
	switch e := event.(type) {
	case *github.IssueCommentEvent:
		if e.Repo != nil {
			return e.Repo.Owner.GetLogin(), e.Repo.GetName()
		}
	case *github.ProjectCardEvent:
		if e.Repo != nil {
			return e.Repo.Owner.GetLogin(), e.Repo.GetName()
		}
	}
	return "", ""
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
	opt := &github.RepositoryContentGetOptions{Ref: "main"}
	_, files, _, err := client.Repositories.GetContents(ctx, owner, repo, ".github/zbot", opt)
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
