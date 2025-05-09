# GitHub Issue bot

- Installed as a GitHub App
- Reads rules from .github/issuebot/*.yaml
- Conditions (`if`) and actions (`run`) are written in lua
- parses events from Webhooks
- can run rules on schedule

Right now the bot doesn't perform any actual call to Github to modify issues, it's just printing the action it would perform

TODO: 
- Implement real GH API calls
- Persist some lightweight state, e.g. for tracking previous assignees or last-seen timestamps (BoltDB, SQLite, or even GitHub issue comments as metadata).
- Extend Lua API with GitHub actions like comment(), reopen_issue(), get_labels(), or list_team_members().
- Mitigate the very strict rate limiting of Github API.
- GitHub API Rate Limit Handling
    - Check remaining rate limit before performing API calls using `client.RateLimits`.
    - Gracefully handle `*github.RateLimitError` by sleeping until `rate.Reset.Time`.
    - Implement exponential or linear backoff with jitter when retrying failed requests.
    - Avoid unnecessary requests by using ETags or `If-Modified-Since` headers.
    - Leverage GitHub App installation-specific limits by processing installations sequentially or with controlled concurrency.


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
