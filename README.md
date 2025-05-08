# GitHub Issue bot

- Installed as a GitHub App
- Reads rules from .github/issuebot/*.yaml
- Conditions (`if`) and actions (`run`) are written in lua
- parses events from Webhooks
- can run rules on schedule


### Example rules

```yaml
rules:
  - name: "Assign to author on Pending Customer"
    event: "project_card:moved"
    if: "event.new_column == 'Pending Customer'"
    run: |
      assign(issue.author)

  - name: "Auto reassign after author reply"
    event: "issue_comment"
    if: "comment.author == issue.author"
    run: |
      assign(issue.last_assignee)

  - name: "Mark stale and close"
    event: "schedule:daily"
    run: |
      if days_open(issue) > 30 and not has_label("stale") then
        label("stale")
      elseif days_open(issue) > 37 then
        close_issue()
      end

  - name: "Tag-based assignment"
    event: "issues:opened"
    run: |
      if string.find(issue.body, '#infra') then
        assign_team_member('infra')
      end
```
