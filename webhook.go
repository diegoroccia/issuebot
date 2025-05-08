package main

import (
	"github.com/google/go-github/v60/github"
	"github.com/yuin/gopher-lua"
	"log"
	"net/http"
	"os"
)

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
