# Rule 1: LiveReview build command

If you need to rebuild LiveReview, just do it via:

bash -lc 'go build livereview.go'

When building the mrmodel command, always use the same binary name "mrmodel"

Start LiveReview in watch mode with "make develop". If you want to start the API server only from a binary "./livereview api". This
by default starts in port 8888. But beofore you do that start postgresql (ensure it is up) with "./pgctl start"

To do db queries you can see the source of "pgctl.sh". Usually, you can do "./pgctl.sh shell -c '<sql command>'" to get results.

When doing refactors or improvements - don't do fallback implementations unless explicitly asked for. Having fallbacks creates very confusing program behavior. Instead of failing, etc  - it leads to highly brittle and annoying program behavior.