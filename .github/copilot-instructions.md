# Rule 1: LiveReview build command

If you need to rebuild LiveReview, just do it via:

bash -lc 'go build livereview.go'

When building the mrmodel command, always use the same binary name "mrmodel"

Start LiveReview in watch mode with "make develop". If you want to start the API server only from a binary "./livereview api". This
by default starts in port 8888. But beofore you do that start postgresql (ensure it is up) with "./pgctl start"

To do db queries you can see the source of "pgctl.sh". Usually, you can do "./pgctl.sh shell -c '<sql command>'" to get results.

When doing refactors or improvements - don't do fallback implementations unless explicitly asked for. Having fallbacks creates very confusing program behavior. Instead of failing, etc  - it leads to highly brittle and annoying program behavior.

When you want to run a test - always run the most specific test you want to run. Don't run all or other irrelevant tests, because there could be failing tests, etc and it makes it hard to see the results of the test you want to run.

But as a whole - you can run "make testall" to run all the relevant tests for the project. When new modules are added - make sure to add them to the "make testall" command.

When making proposals - think of how you can accomplish goals in an incremental way rather than suggesting sweeping refactors.

When you are working on plan files (usually md/markdown files) - avoid writing detailed code (more than 5-10 lines)

Always create and run migrations with dbmate command, not with adhoc sql queries or pgctl.sh, etc. For schema changes always use dbmate migrations.

for building the CLI use the following command:
cd /home/shrsv/bin/LiveReview && make lrc-build-local && lrc hooks install

A primary rule in programming is that the name must match function or meaning. If the name says one thing and the function or meaning is another - it must be treated as a major bug and treated with highest priority. Always fix naming bugs immediately when discovered.