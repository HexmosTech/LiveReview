VSIX := livereview.vsix

.PHONY: package clean install min-api-version

package:
	npm install
	npx @vscode/vsce package -o $(VSIX)

install: package
	code-insiders --install-extension $(VSIX) --force

publish:
	npx @vscode/vsce publish
	npx ovsx publish --pat "ovsxat_80b4f945-4354-4b74-a35e-0a37b74e7500"

# Find the minimum supported VS Code API version by running the probe script
min-api-version:
	./find-min-version.sh

clean:
	rm -f $(VSIX)
