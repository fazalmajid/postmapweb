package main

import (
	"bufio"
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"flag"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/mail"
	"os"
	"os/exec"
	"os/signal"
	"runtime/pprof"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh/terminal"

	"github.com/labstack/echo/v4"
	mw "github.com/labstack/echo/v4/middleware"
)

type Domain struct {
	Name     string
	MapFile  string
	PassHash string
	Script   string
}
type Config struct {
	Domains []Domain
}

var conf Config

//go:embed templates/view.html templates/view.js templates/error.html
var assets embed.FS

//go:embed handsontable static
var staticAssets embed.FS

func View(c echo.Context) error {
	domain := c.Get("domain").(Domain)
	return c.Render(http.StatusOK, "view", struct {
		Domain string
	}{domain.Name})
}

func JS(c echo.Context) error {
	domain := c.Get("domain").(Domain)
	a := readMapFile(domain.MapFile, nil)
	b := make([][]string, 0)
	for i := 0; i < len(a); i++ {
		if strings.Contains(a[i].Email, "@"+domain.Name) {
			b = append(b, []string{a[i].Email, a[i].Target})
		}
	}
	if len(b) == 0 {
		b = append(b, []string{"@" + domain.Name, "nobody"})
	}

	tmpl := c.Echo().Renderer.(Renderer).Template("js")
	var w bytes.Buffer
	err := tmpl.Execute(&w, struct {
		Aliases [][]string
		Domain  string
	}{b, domain.Name})
	if err != nil {
		return err
	}
	if w.Len() < len("<script></script>") {
		return errors.New("Truncated JS template output")
	}
	if string(w.Bytes()[:len("<script>")]) != "<script>" {
		return errors.New("unexpected JS template output")
	}
	return c.Blob(200, "application/javascript", w.Bytes()[len("<script>"):w.Len()-len("</script>")])
}

type ChangeRequest struct {
	Op     string
	Alias  string
	Target string
}

func new_line(label string, fw *bufio.Writer, email string, dest string) {
	pad := 40 - len(email)
	if pad <= 0 {
		pad = 1
	}
	fw.WriteString(email + strings.Repeat(" ", pad) + dest + "\n")
	//log.Println(label, "new_line", email, dest)
}

func validate(target string, local map[string]bool) bool {
	if is_spam(target) {
		return true
	}
	for _, email := range strings.Split(target, ",") {
		email = strings.TrimSpace(email)
		// RFC 5322 parsing/validation of email addresses, accept no inferior
		// regex-based substitute
		_, err := mail.ParseAddress(email)
		switch {
		case err == nil: // valid RFC 5322 address
			continue
		case local[email]: // valid pre-existing local alias
			continue
		default:
			aliases := make([]string, len(local))
			i := 0
			for alias := range local {
				aliases[i] = alias
				i++
			}
			log.Println("attempted alias target", email, "is neither a valid email nor one of the recognized aliases:", strings.Join(aliases, ", "))
			return false
		}
	}
	return true
}

var rewrite_lock sync.Mutex

func is_spam(e string) bool {
	return e == "spam" || e == "SPAM" || strings.HasPrefix(e, "550 ")
}

func Change(c echo.Context) error {
	var changes []ChangeRequest
	domain := c.Get("domain").(Domain)

	if c.FormValue("changes") == "" && c.FormValue("user") != "" && c.FormValue("dest") != "" {
		// quick entry form
		changes = []ChangeRequest{
			{
				"add",
				strings.TrimSpace(c.FormValue("user")) + "@" + domain.Name,
				strings.TrimSpace(c.FormValue("dest")),
			},
		}
	} else {
		err := json.Unmarshal([]byte(c.FormValue("changes")), &changes)
		if err != nil {
			return err
		}
	}
	log.Println("Received changes", changes)

	// serialize map file rewrites
	rewrite_lock.Lock()
	defer func() {
		rewrite_lock.Unlock()
	}()
	// find all existing local addresses, but exclude command delivery
	a := readMapFile(domain.MapFile, nil)
	local := make(map[string]bool)
	for i := 0; i < len(a); i++ {
		if strings.Contains(a[i].Email, "@"+domain.Name) && !strings.ContainsAny(a[i].Target, "@|") {
			for _, dest := range strings.Split(a[i].Target, ",") {
				local[strings.TrimSpace(dest)] = true
			}
		}
	}
	// dedupe and normalize changes
	remap := make(map[string]string)
	for _, change := range changes {
		address, err := mail.ParseAddress(change.Alias)
		if err != nil || !strings.HasSuffix(address.Address, "@"+domain.Name) {
			if change.Alias == "@"+domain.Name {
				address = &mail.Address{Address: change.Alias}
			} else {
				log.Println("invalid alias:", change.Alias)
				return c.Render(http.StatusBadRequest, "error", struct {
					Error string
				}{"invalid alias: " + change.Alias})
			}
		}
		switch change.Op {
		case "remove":
			remap[address.Address] = ""
		case "add":
			if validate(change.Target, local) {
				remap[address.Address] = change.Target
			} else {
				return c.Render(http.StatusBadRequest, "error", struct {
					Error string
				}{"invalid email address list for " + address.Address + ": " + change.Target})
			}
		default:
			log.Fatal("unexpected change request: ", change)
		}
	}
	// rewrite the map file atomically
	tmp_file := domain.MapFile + ".web.new"
	f, err := os.OpenFile(tmp_file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal("could not rewrite map file ", tmp_file, " due to ", err)
	}
	fw := bufio.NewWriter(f)
	// recipient of "spam" goes to its own file
	spam, err := os.OpenFile(tmp_file+".spam", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal("could not rewrite spam map file ", tmp_file+".spam", " due to ", err)
	}
	sw := bufio.NewWriter(spam)
	//log.Println("Preparing to rewrite map files")
	readMapFile(domain.MapFile, func(line string, email string) {
		//log.Printf("RLMF \"%s\" \"%s\" \"%s\"\n", line, email, remap[email])
		if email == "" {
			fw.WriteString(line + "\n")
		} else {
			dest, ok := remap[email]
			if ok {
				switch {
				case dest == "":
					break
				case dest == "spam":
					new_line("SPAM", sw, email, "550 Stop spamming me")
				case dest == "SPAM":
					new_line("SPAM", sw, email, "550 Stop spamming me")
				case strings.HasPrefix(dest, "550 "):
					new_line("SPAM", sw, email, dest)
				default:
					new_line("GOOD", fw, email, dest)
				}
				// remove any further references
				remap[email] = ""
			} else {
				fields := strings.Fields(line)
				dest = strings.Join(fields[1:len(fields)], " ")
				if is_spam(dest) {
					sw.WriteString(line + "\n")
				} else {
					fw.WriteString(line + "\n")
				}
			}
		}
	})
	for email, dest := range remap {
		if dest != "" {
			if is_spam(dest) {
				if !strings.HasPrefix(dest, "550 ") {
					dest = "550 Stop spamming me"
				}
				new_line("SPAM", sw, email, dest)
			} else {
				new_line("GOOD", fw, email, dest)
			}
		}
	}
	fw.Flush()
	sw.Flush()
	f.Close()
	spam.Close()
	err = os.Rename(domain.MapFile, domain.MapFile+".old")
	if err != nil {
		return err
	}

	err = os.Rename(domain.MapFile+".spam", domain.MapFile+".spam.old")

	err = os.Rename(tmp_file, domain.MapFile)
	if err != nil {
		os.Rename(domain.MapFile+".old", domain.MapFile)
		return err
	}
	err = os.Rename(tmp_file+".spam", domain.MapFile+".spam")
	if err != nil {
		os.Rename(domain.MapFile+".spam.old", domain.MapFile+".spam")
		return err
	}

	cmd := exec.Command("postmap", domain.MapFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		log.Println("error running postmap on map file:", err)
		os.Rename(domain.MapFile, domain.MapFile+".bad")
		os.Rename(domain.MapFile+".old", domain.MapFile)
		return err
	}

	cmd = exec.Command("postmap", domain.MapFile+".spam")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		log.Println("error running postmap on spam file:", err)
		os.Rename(domain.MapFile+".spam", domain.MapFile+".spam.bad")
		os.Rename(domain.MapFile+".spam.old", domain.MapFile+".spam")
	}

	cmd = exec.Command("postfix", "reload")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err = cmd.Run()
	if err != nil {
		log.Println("postfix reload failed:", err)
	}

	// optional script hook
	if domain.Script != "" {
		cmd = exec.Command(domain.Script, domain.Name)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err = cmd.Run()
		if err != nil {
			log.Println("script", domain.Script, "failed:", err)
		}
	}

	// HTTP 303 is specifically for POST/Redirect/GET
	// see: https://en.wikipedia.org/wiki/Post/Redirect/Get
	c.Response().Header().Set("Location", "/")
	return c.HTML(303, "<script>document.location.href = \"/\";</script>")
}

func readConf(conf_file string) Config {
	conf_stat, err := os.Stat(conf_file)
	if err != nil {
		log.Println("could not stat conf file", conf_file, "due to", err, "- assuming an empty config file")
		return Config{}
	}
	conf_size := int(conf_stat.Size())
	f, err := os.Open(conf_file)
	if err != nil {
		log.Fatal("could not open conf file ", conf_file, " due to ", err)
	}
	conf_data := make([]byte, conf_size)
	bytes_read, err := f.Read(conf_data)
	if err != nil || bytes_read < conf_size {
		log.Fatal("could not read conf file ", conf_file, " error: ", err, " read ", bytes_read, " bytes ")
	}
	var conf Config
	err = json.Unmarshal(conf_data, &conf)
	if err != nil || bytes_read < conf_size {
		log.Fatal("could not decode conf file ", conf_file, " due to ", err)
	}
	if *verbose {
		log.Println("config file:", conf)
	}
	return conf
}

func updateConf(conf Config, conf_file string, domain string, virtual string, password []byte) {
	pass_hash, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		log.Fatal("Could not hash the password: ", err)
	}
	// if the domain already exists in the config file,
	// change the password and map file
	existing := false
	for i, d := range conf.Domains {
		if d.Name == domain {
			existing = true
			conf.Domains[i].MapFile = virtual
			conf.Domains[i].PassHash = string(pass_hash)
		}
	}
	if !existing {
		conf.Domains = append(conf.Domains, Domain{domain, virtual, string(pass_hash), ""})
	}
	conf_json, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		log.Fatal("Could not marshal the config: ", err)
	}
	f, err := os.OpenFile(conf_file, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal("could not write conf file ", conf_file, " due to ", err)
	}
	nbytes, err := f.Write(conf_json)
	if err != nil || nbytes < len(conf_json) {
		log.Fatal("could not write conf file ", conf_file, " due to ", err)
	}
	err = f.Close()
	if err != nil {
		log.Fatal("could not finish writing conf file ", conf_file, " due to ", err)
	}
}

type Alias struct {
	Email  string
	Target string
}

func readMapFile(map_file string, hook func(line string, email string)) []Alias {
	good, err := readSingleMapFile(map_file, hook)
	if err != nil {
		log.Fatal("could not open map file ", map_file, " due to ", err)
	}
	spam, err := readSingleMapFile(map_file+".spam", hook)
	if err == nil {
		good = append(good, spam...)
	}
	return good
}

func readSingleMapFile(map_file string, hook func(line string, email string)) ([]Alias, error) {
	f, err := os.Open(map_file)
	if err != nil {
		return nil, err
	}
	aliases := make([]Alias, 0)
	scanner := bufio.NewScanner(bufio.NewReader(f))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			if hook != nil {
				hook(line, "")
			}
			continue
		}
		fields := strings.Fields(line)
		if strings.HasSuffix(map_file, ".spam") {
			aliases = append(aliases, Alias{fields[0], strings.Join(fields[1:len(fields)], " ")})
			if hook != nil {
				hook(line, fields[0])
			}
		} else {
			// there could be a comment after the map entry
			if len(fields) >= 2 && strings.ContainsRune(fields[0], '@') {
				if strings.ContainsRune(fields[0], '@') {
					aliases = append(aliases, Alias{fields[0], fields[1]})
				}
				if hook != nil {
					hook(line, fields[0])
				}
			} else {
				if hook != nil {
					hook(line, "")
				}
			}
		}
	}
	return aliases, nil
}

type Renderer struct {
	templates map[string]*template.Template
}

func (r *Renderer) Load() {
	r.templates = map[string]*template.Template{
		"view":  r.parse("view.html", "view"),
		"js":    r.parse("view.js", "js"),
		"error": r.parse("error.html", "error"),
	}
}
func (r Renderer) Template(name string) *template.Template {
	return r.templates[name]
}
func (r *Renderer) parse(filename string, name string) *template.Template {
	templateBytes, err := assets.ReadFile("templates/" + filename)
	if err != nil {
		log.Fatal(err)
	}
	templateString := string(templateBytes)
	if strings.HasSuffix(filename, ".js") {
		// recipe from: https://damienradtke.com/post/go-js-template/
		templateString = "<script>" + templateString + "</script>"
	}
	tmpl, err := template.New(name).Parse(templateString)
	if err != nil {
		log.Fatal(err)
	}
	return tmpl
}
func (r Renderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	tmpl := r.templates[name]
	return tmpl.Execute(w, data)
}

var verbose *bool

func main() {
	// command-line args parsing
	conf_file := flag.String("c", "/etc/postfix/postmapweb.json", "config file to use")
	verbose = flag.Bool("v", false, "verbose logging")
	domain := flag.String("d", "", "add domain user")
	virtual := flag.String("m", "/etc/postfix/virtual", "virtual domain map to use with -d")
	cl_password := flag.String("w", "", "password to use with -d (insecure!)")
	cpuprofile := flag.String("cpuprofile", "", "write cpu profile to file")
	port := flag.String("p", "localhost:8080", "host address and port to bind to")
	flag.Parse()
	conf = readConf(*conf_file)

	if *domain != "" {
		password := []byte(*cl_password)
		if len(password) == 0 {
			os.Stdout.Write([]byte("Enter password for " + *domain + ":"))
			password1, err := terminal.ReadPassword(0)
			if err != nil {
				log.Fatal("password error: ", err)
			}
			os.Stdout.Write([]byte("\nConfirm password:"))
			password2, err := terminal.ReadPassword(0)
			os.Stdout.Write([]byte("\n"))
			if err != nil {
				log.Fatal("password error: ", err)
			}
			if string(password1) != string(password2) {
				log.Fatal("the passwords do not match")
			}
			password = password1
		}
		//log.Println("read password:", "\"" + string(password) + "\"")

		updateConf(conf, *conf_file, *domain, *virtual, password)
		log.Println("updated conf", *conf_file)
		return
	}

	// profiler
	if *cpuprofile != "" {
		f, err := os.Create(*cpuprofile)
		if err != nil {
			log.Fatal(err)
		}
		log.Println("starting CPU profile")
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGHUP)
		go func() {
			s := <-sigChan
			log.Println("stopping CPU profile due to", s)
			pprof.StopCPUProfile()
		}()
	}

	// serve go:embed embedded assets
	assetHandler := http.FileServer(http.FS(staticAssets))

	// Echo instance
	e := echo.New()
	e.HideBanner = true

	r := Renderer{}
	r.Load()
	e.Renderer = r

	// Middleware
	if *verbose {
		e.Use(mw.Logger())
	}
	e.Use(mw.Recover())
	e.Use(SecurityHeaders)
	e.Use(BasicAuth)
	//e.Use(mw.Gzip())

	//e.Favicon("img/favicon.ico")

	// embedded static assets
	e.GET("/handsontable/*", func(c echo.Context) error {
		assetHandler.ServeHTTP(c.Response().Writer, c.Request())
		return nil
	})
	e.GET("/static/*", func(c echo.Context) error {
		assetHandler.ServeHTTP(c.Response().Writer, c.Request())
		return nil
	})
	e.GET("/", View)
	e.GET("/view.js", JS)
	e.POST("/", Change)

	// Start server
	log.Println("starting postmapweb on", *port)
	e.Start(*port)
}
