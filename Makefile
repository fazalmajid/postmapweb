GO=	env GOPATH=`pwd` go

all: run
run: postmapweb $(HANDSONTABLE) conf.json
	./postmapweb -v -c conf.json
conf.json:
	echo '{"domains": []}' > conf.json

GODEPS=	src/github.com/labstack/echo \
	src/golang.org/x/crypto/bcrypt
GODEPS_PATCHED=	src/golang.org/x/crypto/ssh/terminal

postmapweb: postmapweb.go $(GODEPS_PATCHED) $(GODEPS) $(HANDSONTABLE)
	$(GO) build postmapweb.go

$(GODEPS):
	$(GO) get ${@:src/%=%}

src/golang.org/x/crypto/ssh/terminal:
	-mkdir -p src/golang.org/x
	git clone -q https://go.googlesource.com/crypto src/golang.org/x/crypto
	cp patches/util_solaris.go src/golang.org/x/crypto/ssh/terminal
	$(GO) get golang.org/x/crypto/ssh/terminal

BOWER=	node_modules/bower/bin/bower
$(BOWER):
	npm install bower

HANDSONTABLE=	bower_components/handsontable/.bower.json
$(HANDSONTABLE): $(BOWER)
	echo n | ./node_modules/bower/bin/bower install handsontable --save

clean:
	-rm -f postmapweb
	-rm -rf node_modules pkg src bower_modules
	-rm -f *~ */*~
