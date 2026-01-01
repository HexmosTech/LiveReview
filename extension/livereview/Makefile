VSIX := livereview.vsix

.PHONY: package clean

package:
	npm install
	npx @vscode/vsce package -o $(VSIX)

clean:
	rm -f $(VSIX)
