package main

import (
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/google/go-github/v60/github"
	"github.com/yuin/gopher-lua"
)

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, []byte(githubWebhookSecret))
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
		log.Println("Assigning to", user)
		return 0
	}))

	L.SetGlobal("label", L.NewFunction(func(L *lua.LState) int {
		label := L.ToString(1)
		log.Println("Adding label", label)
		return 0
	}))

	L.SetGlobal("close_issue", L.NewFunction(func(L *lua.LState) int {
		log.Println("Closing issue")
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
		if len(e.Issue.Labels) > 0 {
			var labels lua.LTable
			for _, label := range e.Issue.Labels {
				labels.Append(lua.LString(label.GetName()))
			}
			tbl.RawSetString("labels", &labels)
		}
	case *github.ProjectCardEvent:
		tbl.RawSetString("type", lua.LString("project_card:moved"))
		if e.ProjectCard != nil {
			tbl.RawSetString("new_column", lua.LString(e.ProjectCard.GetColumnName()))
		}
		// Extract content URL and parse it
		if e.ProjectCard.ContentURL != nil {
			contentURL := *e.ProjectCard.ContentURL
			u, err := url.Parse(contentURL)
			if err != nil {
				log.Fatalf("Error parsing content URL: %v", err)
			}

			// Example URL format: https://api.github.com/repos/{owner}/{repo}/issues/{issue_number}
			pathParts := splitPath(u.Path) if len(pathParts) < 5 || pathParts[3] != "issues" {
				log.Fatalf("Content URL does not point to an issue: %s", contentURL)
			}

			owner := pathParts[1]
			repo := pathParts[2]
			issueNumber, err := strconv.Atoi(pathParts[4])
			if err != nil {
				log.Fatalf("Error converting issue number: %v", err)
			}

			// Get the issue
			issue, _, err := client.Issues.Get(ctx, owner, repo, issueNumber)
			if err != nil {
				log.Fatalf("Error fetching issue: %v", err)
			}

			fmt.Printf("Issue Title: %s\n", *issue.Title)
			fmt.Printf("Issue Body: %s\n", *issue.Body)
		} else {
			log.Println("No associated issue found for the project card.")
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
