package main

import (
    "net/http"
	"html/template"
	"io"
	"log"
	"flag"
	"os"
	"os/exec"
	"os/signal"
	"encoding/json"
	"bufio"
	"strings"
	"encoding/base64"
	"sync"
	"net/mail"
	"syscall"
	"runtime/pprof"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh/terminal"

    "github.com/labstack/echo"
    mw "github.com/labstack/echo/middleware"
	"github.com/GeertJohan/go.rice"
)
type Domain struct {
	Name string
	MapFile string
	PassHash string
}
type Config struct {
	Domains []Domain
}
var conf Config

// BasicAuth returns an HTTP basic authentication middleware.
//
// For valid credentials it calls the next handler.
// For invalid credentials, it sends "401 - Unauthorized" response.
const (
        Basic = "Basic"
)
func BasicAuth() echo.HandlerFunc {
	return func(c *echo.Context) error {
		// Skip WebSocket
		if (c.Request().Header.Get(echo.Upgrade)) == echo.WebSocket {
			return nil
		}

		auth := c.Request().Header.Get(echo.Authorization)
		l := len(Basic)

		if len(auth) > l+1 && auth[:l] == Basic {
			b, err := base64.StdEncoding.DecodeString(auth[l+1:])
			if err == nil {
				cred := string(b)
				for i := 0; i < len(cred); i++ {
					if cred[i] == ':' {
						// Verify credentials
						for _, d := range conf.Domains {
							if cred[:i] == d.Name && bcrypt.CompareHashAndPassword([]byte(d.PassHash), []byte(cred[i+1:])) == nil {
								c.Set("domain", d)
								return nil
							}
						}
					}
				}
			}
		}
		c.Response().Header().Set(echo.WWWAuthenticate, Basic+" realm=Restricted")
		return echo.NewHTTPError(http.StatusUnauthorized)
	}
}

func View(c *echo.Context) error {
	domain := c.Get("domain").(Domain)
	a := readMapFile(domain.MapFile, nil)
	b := make([][]string, 0)
	for i:=0; i<len(a); i++ {
		if strings.Contains(a[i].Email, "@" + domain.Name) {
			b = append(b, []string{a[i].Email, a[i].Target})
		}
	}
	if len(b) == 0 {
		b = append(b, []string{"@" + domain.Name, "nobody"})
	}
    return c.Render(http.StatusOK, "view", struct {
		Aliases [][]string
		Domain string
	}{b, domain.Name})
}
type ChangeRequest struct {
	Op string
	Alias string
	Target string
}

func new_line(fw *bufio.Writer, email string, dest string) {
	pad := 40 - len(email)
	if pad <= 0 {
		pad = 1
	}
	fw.WriteString(email + strings.Repeat(" ", pad) + dest + "\n")
}

func validate(target string, local map[string]bool) bool {
	for _, email := range(strings.Split(target, ",")) {
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
			for alias := range(local) {
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
func Change(c *echo.Context) error {
	var changes []ChangeRequest
	err := json.Unmarshal([]byte(c.Form("changes")), &changes)
	if err != nil {
		return err
	}
	log.Println("Received changes", changes)
	domain := c.Get("domain").(Domain)

	// serialize map file rewrites
	rewrite_lock.Lock()
	defer func() {
		rewrite_lock.Unlock()
	}();
	// find all existing local addresses, but exclude command delivery
	a := readMapFile(domain.MapFile, nil)
	local := make(map[string]bool)
	for i:=0; i<len(a); i++ {
		if strings.Contains(a[i].Email, "@" + domain.Name) && !strings.ContainsAny(a[i].Target, "@|") {
			local[a[i].Target] = true
		}
	}
	// dedupe and normalize changes
	remap := make(map[string]string)
	for _, change := range(changes){
		address, err := mail.ParseAddress(change.Alias)
		if err != nil || strings.HasSuffix(address.Address, "@" + domain.Name) {
			log.Println("invalid alias:", change.Alias)
			return c.Render(http.StatusBadRequest, "error", struct {
				Error string
			}{"invalid alias: " + change.Alias})
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
	f, err := os.OpenFile(tmp_file, os.O_RDWR | os.O_CREATE | os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal("could not rewrite map file ", tmp_file, " due to ", err)
	}
	fw := bufio.NewWriter(f)
	readMapFile(domain.MapFile, func(line string, email string) {
		if email == "" {
			fw.WriteString(line + "\n")
		} else {
			dest, ok := remap[email]
			if ok {
				if dest != "" {
					new_line(fw, email, dest)
					// remove any further references
					remap[email] = ""
				}
			} else {
				fw.WriteString(line + "\n")
			}
		}
	})
	for email, dest := range(remap) {
		new_line(fw, email, dest)
	}
	fw.Flush()
	f.Close()
	err = os.Rename(domain.MapFile, domain.MapFile + ".old")
	if err != nil {
		return err
	}
	err = os.Rename(tmp_file, domain.MapFile)
	if err != nil {
		os.Rename(domain.MapFile + ".old", domain.MapFile)
		return err
	}
	cmd := exec.Command("postmap", domain.MapFile)
	err = cmd.Run()
	if err != nil {
		os.Rename(domain.MapFile + ".old", domain.MapFile)
		return err
	}
	cmd = exec.Command("postfix", "reload")
	err = cmd.Run()
	if err != nil {
		log.Println("postfix reload failed:", err)
	}

	// response
	return View(c)
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
		conf.Domains = append(conf.Domains, Domain{domain, virtual, string(pass_hash)})
	}
	conf_json, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		log.Fatal("Could not marshal the config: ", err)
	}
	f, err := os.OpenFile(conf_file, os.O_RDWR | os.O_CREATE | os.O_TRUNC, 0644)
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
	Email string
	Target string
}
func readMapFile(map_file string, hook func(line string, email string)) []Alias {
	f, err := os.Open(map_file)
	if err != nil {
		log.Fatal("could not open map file ", map_file, " due to ", err)
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
		// there could be a comment after the map entry
		if len(fields) >= 2 && strings.ContainsRune(fields[0], '@'){ 
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
	return aliases
}

type Renderer struct {
	box *rice.Box
	templates map[string]*template.Template
}
func (r *Renderer) Load() {
	box, err := rice.FindBox("templates")
	if err != nil {
		log.Fatal(err)
	}
	r.box = box
	r.templates = map[string]*template.Template{
		"view": r.parse("view.html", "view"),
		"error": r.parse("error.html", "error"),
	}
}
func (r *Renderer) parse(filename string, name string) *template.Template {
	templateString, err := r.box.String(filename)
	if err != nil {
		log.Fatal(err)
	}
	tmpl, err := template.New(name).Parse(templateString)
	if err != nil {
		log.Fatal(err)
	}
	return tmpl
}
func (r Renderer) Render(w io.Writer, name string, data interface{}) error {
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

	// go.rice embedded assets
	assetHandler := http.FileServer(rice.MustFindBox("handsontable").HTTPBox())

	// Echo instance
	e := echo.New()

	r := Renderer{}
	r.Load()
	e.SetRenderer(r)
	
	// Middleware
	if *verbose {
		e.Use(mw.Logger())
	}
	e.Use(mw.Recover())
	e.Use(BasicAuth())
	//e.Use(mw.Gzip())

	//e.Favicon("img/favicon.ico")
	// embedded static assets
	e.Get("/handsontable/*", func(c *echo.Context) error {
		http.StripPrefix("/handsontable/", assetHandler).
			ServeHTTP(c.Response().Writer(), c.Request())
		return nil
	})
	e.Get("/", View)
	e.Post("/", Change)

	// Start server
	log.Println("starting postmapweb on", *port)
	e.Run(*port)
}
