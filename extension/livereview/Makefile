-include .env
export OVSX_PAT

VSIX := livereview.vsix

.PHONY: package clean install min-api-version

package:
	npm install
	npx @vscode/vsce package -o $(VSIX)

install: package
	code-insiders --install-extension $(VSIX) --force

publish:
	npx @vscode/vsce publish
	npx ovsx publish --pat $(OVSX_PAT)

# Find the minimum supported VS Code API version by running the probe script
min-api-version:
	./find-min-version.sh

clean:
	rm -f $(VSIX)
