package main

import (
    "net/http"
	"html/template"
	"io"
	"log"
	"flag"
	"os"
	"os/exec"
	"encoding/json"
	"bufio"
	"strings"
	"encoding/base64"
	"sync"
	"net/mail"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/ssh/terminal"

    "github.com/labstack/echo"
    mw "github.com/labstack/echo/middleware"
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
type Template struct {
    templates *template.Template
}
func (t *Template) Render(w io.Writer, name string, data interface{}) error {
    return t.templates.ExecuteTemplate(w, name, data)
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
		log.Println("postfix reload failed: ", err)
	}

	// response
	return View(c)
}

func readConf(conf_file string) Config {
	conf_stat, err := os.Stat(conf_file)
	if err != nil {
		log.Fatal("could not stat conf file ", conf_file, " due to ", err)
	}
	conf_size := int(conf_stat.Size())
	f, err := os.Open(conf_file)
	if err != nil {
		log.Fatal("could not open conf file ", conf_file, " due to ", err)
	}
	conf_data := make([]byte, conf_size)
	bytes_read, err := f.Read(conf_data)
	if err != nil || bytes_read < conf_size {
		log.Fatal("could not read conf file ", conf_file, " error: ", err, " read ", bytes_read, " bytes")
	}
	var conf Config
	err = json.Unmarshal(conf_data, &conf)
	if err != nil || bytes_read < conf_size {
		log.Fatal("could not decode conf file ", conf_file, " due to ", err)
	}
	if *verbose {
		log.Println("config file: ", conf)
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

var verbose *bool
func main() {
	// command-line args parsing
	conf_file := flag.String("c", "/etc/postfix/postmapweb.json", "config file to use")	
	verbose = flag.Bool("v", false, "verbose logging")	
	domain := flag.String("d", "", "add domain (requires -p and optionally -v)")
	virtual := flag.String("m", "/etc/postfix/virtual", "virtual domain map to use with -d")	
	flag.Parse()
	conf = readConf(*conf_file)

	if *domain != "" {
		os.Stdout.Write([]byte("Enter password for " + *domain + ":"))
		password, err := terminal.ReadPassword(0)
		if err != nil {
			log.Fatal("password error: ", err)
		}
		os.Stdout.Write([]byte("\nConfirm password:"))
		password2, err := terminal.ReadPassword(0)
		os.Stdout.Write([]byte("\n"))
		if err != nil {
			log.Fatal("password error: ", err)
		}
		if string(password2) != string(password) {
			log.Fatal("the passwords do not match")
		}
		log.Println("read password: ", string(password))
		updateConf(conf, *conf_file, *domain, *virtual, password)
		log.Println("updated conf")
		return
	}
	// templates
	t := &Template{
		templates: template.Must(template.ParseGlob("templates/*.html")),
	}
	// Echo instance
	e := echo.New()
	e.SetRenderer(t)
	
	// Middleware
	e.Use(mw.Logger())
	e.Use(mw.Recover())
	e.Use(BasicAuth())
	//e.Use(mw.Gzip())

	// Routes
	e.Static("/css/", "css")
	e.Static("/js/", "js")
	e.Static("/img/", "img")
	e.Static("/handsontable/", "bower_components/handsontable")
	e.Favicon("img/favicon.ico")
	e.Get("/", View)
	e.Post("/", Change)

	// Start server
	log.Println("starting postmapweb on port 8080")
	e.Run(":8080")
}
