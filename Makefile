GO=	env GOPATH=`pwd` go

all: postmapweb
run: postmapweb
	./postmapweb -v -p :8080 -c conf.json

GODEPS=	src/github.com/labstack/echo \
	src/golang.org/x/crypto/bcrypt \
	src/github.com/GeertJohan/go.rice
GODEPS_PATCHED=	src/golang.org/x/crypto/ssh/terminal
CSS=	handsontable/dist/handsontable.full.min.css
JS=	handsontable/dist/handsontable.full.min.js
ASSETS=	$(CSS) $(JS) templates/view.html templates/error.html
TEMPLATES= templates/view.html templates/error.html
RICE=	templates.rice-box.go handsontable.rice-box.go
templates.rice-box.go: $(TEMPLATES) $(CSS) $(JS) bin/rice
	bin/rice embed-go
handsontable.rice-box.go: $(CSS) $(JS) bin/rice
	bin/rice embed-go

postmapweb: postmapweb.go $(GODEPS_PATCHED) $(GODEPS) $(RICE)
	$(GO) build postmapweb.go $(RICE)

bin/rice:
	$(GO) get -u github.com/GeertJohan/go.rice
	$(GO) get -u github.com/GeertJohan/go.rice/rice

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

HANDSONTABLE=	bower_components/handsontable/dist
$(JS) $(CSS): $(BOWER)
	echo n | ./node_modules/bower/bin/bower install handsontable --save
	mkdir -p handsontable/dist
	cp $(HANDSONTABLE)/handsontable.full.min.* handsontable/dist

profile: cpu.prof
	echo top10|go tool pprof postmapweb cpu.prof
cpu.pdf: cpu.prof
	echo 'pdf > cpu.pdf'|go tool pprof postmapweb cpu.prof
cpu.prof: postmapweb
	-rm -f virtual.cpu
	touch virtual.cpu
	./postmapweb -p :8080 -c conf.cpu -d temboz.com -m virtual.cpu -w sopo
	./postmapweb -p :8080 -c conf.cpu -cpuprofile cpu.prof &
	sleep 3
	ab -n 10 -A temboz.com:sopo 'http://localhost:8080/handsontable/handsontable.full.min.css'
#	ab -n 10 -A temboz.com:sopo 'http://localhost:8080/handsontable/handsontable.full.min.js'
#	ab -n 10 -A temboz.com:sopo 'http://localhost:8080/'
	pkill -HUP postmapweb
	sleep 1
	pkill -9 postmapweb

profileclean:
	-rm -f *.cpu cpu.*

clean: profileclean
	-rm -f postmapweb bindata.go bin/rice *.rice-box.go
	-rm -rf node_modules pkg src bower_* handsontable
	-rm -f *~ */*~ ._*
