GO=	go

all: postmapweb
run: postmapweb
	./postmapweb -v -p :8080 -c conf.json

CSS=	handsontable/dist/handsontable.full.min.css
JS=	handsontable/dist/handsontable.full.min.js templates/view.js
ASSETS=	$(CSS) $(JS) templates/view.html templates/error.html
TEMPLATES= templates/view.html templates/error.html

postmapweb: postmapweb.go middleware.go $(JS) $(CSS)
	$(GO) build

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
$(JS) $(CSS):
	$(MAKE) $(BOWER)
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

test: postmapweb
	-mkdir -p test
	/usr/bin/echo "sentfrom.com\tdomain" > test/virtual
	/usr/bin/echo "something@sentfrom.com\tmajid" >> test/virtual
	./postmapweb -c test/conf.json -d sentfrom.com -m test/virtual
testrun: test
	./postmapweb -v -p :8081 -c test/conf.json

profileclean:
	-rm -f *.cpu cpu.*

clean: profileclean
	-chmod -R u+rwx pkg
	-rm -rf pkg
	-rm -f postmapweb bindata.go bin/rice *.rice-box.go
	-rm -rf node_modules pkg src bower_*
	-rm -rf test
	-rm -f *~ */*~ ._*
