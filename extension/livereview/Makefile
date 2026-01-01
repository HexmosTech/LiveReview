VSIX := livereview.vsix

.PHONY: package clean install

package:
	npm install
	npx @vscode/vsce package -o $(VSIX)

install: package
	code-insiders --install-extension $(VSIX)

publish:
	npx @vscode/vsce publish

clean:
	rm -f $(VSIX)
