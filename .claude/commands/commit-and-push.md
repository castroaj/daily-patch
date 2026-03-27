Write a comprehensive commit message for any staged changes, then commit and push. The commit message should:

1. Have a concise subject line (under 72 characters) summarizing the change
2. Include a detailed body explaining *what* changed and *why*
3. End with the `Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>` trailer

## Steps

1. Run `git status` to see staged, unstaged, and untracked files
2. Run `git diff --cached` to inspect exactly what is staged
3. Run `git log --oneline -5` to match the repository's commit message style
4. **Show the user** a summary of the files being committed (table with file, change type, and description) and the full proposed commit message — ask for confirmation before proceeding
5. Run `git commit` with the approved message
6. Run `git push` to the current tracking branch

## Additional context provided by the user

$ARGUMENTS
