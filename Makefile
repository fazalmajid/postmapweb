GO=	go

all: postmapweb
run: postmapweb
	./postmapweb -v -p :8080 -c conf.json

CSS=	gridjs/dist/theme/mermaid.min.css
JS=	gridjs/dist/gridjs.umd.js templates/view.js
ASSETS=	$(CSS) $(JS) templates/view.html templates/error.html
TEMPLATES= templates/view.html templates/error.html

postmapweb: postmapweb.go middleware.go $(ASSETS)
	$(GO) build

gridjs/dist/theme/mermaid.min.css:
	mkdir -p gridjs/dist/theme
	curl -L -o gridjs/dist/theme/mermaid.min.css \
		'https://cdn.jsdelivr.net/npm/gridjs/dist/theme/mermaid.min.css'

gridjs/dist/gridjs.umd.js:
	mkdir -p gridjs/dist
	curl -L -o gridjs/dist/gridjs.umd.js \
		'https://cdn.jsdelivr.net/npm/gridjs/dist/gridjs.umd.js'

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
	ab -n 10 -A temboz.com:sopo 'http://localhost:8080/gridjs/dist/theme/mermaid.min.css'
#	ab -n 10 -A temboz.com:sopo 'http://localhost:8080/gridjs/dist/gridjs.umd.js'
#	ab -n 10 -A temboz.com:sopo 'http://localhost:8080/'
	pkill -HUP postmapweb
	sleep 1
	pkill -9 postmapweb

test: postmapweb
	-mkdir -p test
	echo "sentfrom.com:domain" | tr : '\t' > test/virtual
	echo "something@sentfrom.com:majid" | tr : '\t' >> test/virtual
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
	-rm -rf gridjs
	-rm -rf test
	-rm -f *~ */*~ ._*
