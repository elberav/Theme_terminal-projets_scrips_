package main

import (
	"context" // Nuevo: para timeouts en comandos
	"flag"
	"fmt"
	"os" // Reemplaza a "io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// --- 1. Definición de Colores (Sin Shell-Specific Wrapping) ---
const (
	// Códigos ANSI puros (sin \[\] o %{%})
	ResetCode      = "\x1b[0m"
	WhiteCode      = "\x1b[97m"
	BlackCode      = "\x1b[38;5;16m"
	CBlueCode      = "\x1b[38;2;107;189;183m"
	CCyanCode      = "\x1b[38;5;44m"
	CGreenCode     = "\x1b[38;5;113m" // Verde (Activo Físico)
	CYellowCode    = "\x1b[38;5;178m" // Amarillo (Virtual / Pyenv)
	COrangeCode    = "\x1b[38;5;214m" // Naranja (Git con cambios sin commit)
	CPurpleCode    = "\x1b[38;5;147m" // Morado/Lavanda (Git con commits sin push)
	CLightBlueCode = "\x1b[38;5;117m" // Azul Claro (Staged / "add ." hecho)
	CPinkCode      = "\x1b[38;5;225m" // Rosa (Behind / Cambios en nube)

	CRedCode     = "\x1b[38;5;203m"
	CGrayTxtCode = "\x1b[38;5;250m" // Gris (Inactivo)

	// Fondos
	BgMainCode = "\x1b[48;5;236m"
	BgTopCode  = "\x1b[48;5;236m"
	BgBlueCode = "\x1b[48;2;107;189;183m"

	FgMainColorCode = "\x1b[38;5;236m"
	FgBlueColorCode = "\x1b[38;2;107;189;183m"
)

// Variables globales para colores envueltos según shell
var (
	Reset       string
	White       string
	Black       string
	CBlue       string
	CCyan       string
	CGreen      string
	CYellow     string
	COrange     string
	CPurple     string
	CLightBlue  string
	CPink       string
	CRed        string
	CGrayTxt    string
	BgMain      string
	BgTop       string
	BgBlue      string
	FgMainColor string
	FgBlueColor string
)

// --- Iconos ---
const (
	IconOS        = ""
	IconDir       = ""
	IconGit       = ""
	IconGitAdd    = "!" // Falta Add (Unstaged)
	IconGitCommit = "+" // Falta Commit (Staged)
	IconGitPush   = "⇡" // Falta Push (Ahead)
	IconGitPull   = "⇣" // Falta Pull (Behind)
	IconPy        = "" // Icono genérico Python
	IconVenv      = "" // Icono para venv físicos (cajas)
	IconVirt      = "" // Icono para pyenv/managers (serpiente)
	IconK8s       = ""
	IconNode      = ""
	IconDocker    = ""
	IconTime      = ""
	IconRust      = ""
	IconRuby      = ""
	IconPHP       = ""
	IconJava      = ""
	SepArrow      = ""
	SepR          = ""
	SepL          = ""
	SepLine       = ""
)

// Timeout para comandos externos
const commandTimeout = 500 * time.Millisecond
const cacheTTL = 2 * time.Second // Tiempo de vida de la caché

// Configuración global
type GlobalConfig struct {
	DebugMode  bool
	NoIcons    bool
	ShellType  string // "bash", "zsh", "fish", "other"
	InstallCmd bool   // Generar script de instalación
}

var AppConfig GlobalConfig

// CacheEntry almacena el valor y el tiempo de expiración
type CacheEntry struct {
	Value      string
	Expiration time.Time
}

// GlobalCache para almacenar resultados de detecciones
var globalCache = struct {
	sync.RWMutex
	data map[string]CacheEntry
}{
	data: make(map[string]CacheEntry),
}

// DirectoryCache almacena el listado de archivos del directorio actual
var directoryCache = struct {
	sync.RWMutex
	files      []string
	dirs       []string
	expiration time.Time
}{}

// ContextData almacena la información del contexto del prompt de forma thread-safe
type ContextData struct {
	mu     sync.RWMutex
	Git    string
	Python string
	Node   string
	K8s    string
	Docker string
	Rust   string
	Ruby   string
	PHP    string
	Java   string
}

// Métodos thread-safe para ContextData
func (c *ContextData) SetGit(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Git = val
}

func (c *ContextData) SetPython(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Python = val
}

func (c *ContextData) SetNode(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Node = val
}

func (c *ContextData) SetK8s(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.K8s = val
}

func (c *ContextData) SetDocker(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Docker = val
}

func (c *ContextData) SetRust(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Rust = val
}

func (c *ContextData) SetRuby(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Ruby = val
}

func (c *ContextData) SetPHP(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.PHP = val
}

func (c *ContextData) SetJava(val string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Java = val
}

func (c *ContextData) GetAll() (git, python, node, k8s, docker, rust, ruby, php, java string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Git, c.Python, c.Node, c.K8s, c.Docker, c.Rust, c.Ruby, c.PHP, c.Java
}

func main() {
	// Parse command line flags
	var noIcons bool
	var installCmd bool
	var shellType string

	flag.BoolVar(&noIcons, "no-icons", false, "Deshabilitar iconos Nerd Font")
	flag.BoolVar(&installCmd, "install", false, "Generar script de instalación")
	flag.StringVar(&shellType, "shell", "", "Forzar tipo de shell: bash, zsh, fish (auto-detecta si no se especifica)")
	flag.Parse()

	// Inicializar configuración
	initAppConfig(noIcons, installCmd, shellType)

	// Si se pidió instalación, generar script y salir
	if AppConfig.InstallCmd {
		generateInstallScript()
		return
	}

	// Inicializar colores según shell detectada
	initColors()

	// Cargar caché de directorio para optimizar detecciones
	loadDirectoryCache()

	// Obtener código de salida del último comando
	lastExitCode := "0"
	args := flag.Args()
	if len(args) > 0 {
		lastExitCode = args[0]
	}

	var wg sync.WaitGroup
	data := &ContextData{} // Usamos puntero a ContextData para pasarla a las goroutines

	// Ejecutar todas las detecciones en paralelo con goroutines thread-safe y caché
	wg.Add(9) // Ahora hay más detecciones
	go func() { defer wg.Done(); data.SetGit(getGitStatus()) }()
	go func() { defer wg.Done(); data.SetPython(getPythonEnv()) }()
	go func() { defer wg.Done(); data.SetNode(getNodeVersion()) }()
	go func() { defer wg.Done(); data.SetK8s(getK8sContext()) }()
	go func() { defer wg.Done(); data.SetDocker(getDockerStatus()) }()
	go func() { defer wg.Done(); data.SetRust(getRustStatus()) }()
	go func() { defer wg.Done(); data.SetRuby(getRubyStatus()) }()
	go func() { defer wg.Done(); data.SetPHP(getPhpStatus()) }()
	go func() { defer wg.Done(); data.SetJava(getJavaStatus()) }()

	wg.Wait()

	// Obtener todos los valores de forma thread-safe
	git, python, node, k8s, docker, rust, ruby, php, java := data.GetAll()

	// --- Top Content ---
	var parts []string
	if git != "" {
		parts = append(parts, git)
	}
	if python != "" {
		parts = append(parts, python)
	}
	if node != "" {
		parts = append(parts, node)
	}
	if k8s != "" {
		parts = append(parts, k8s)
	}
	if docker != "" {
		parts = append(parts, docker)
	}
	if rust != "" {
		parts = append(parts, rust)
	}
	if ruby != "" {
		parts = append(parts, ruby)
	}
	if php != "" {
		parts = append(parts, php)
	}
	if java != "" {
		parts = append(parts, java)
	}

	// Hora
	timeVal := time.Now().Format("15:04:05")
	timeIcon := getIcon(IconTime)
	if timeIcon != "" {
		timeIcon += " "
	}
	parts = append(parts, fmt.Sprintf("%s%s%s", White, timeIcon, timeVal))

	topContent := strings.Join(parts, fmt.Sprintf(" %s%s ", CGrayTxt, SepLine))

	// --- Bottom Content ---
	cwd, err := os.Getwd()
	if err != nil {
		debugLog("ERROR: No se pudo obtener el directorio actual: %v", err)
		cwd = "?"
	}

	home, err := os.UserHomeDir()
	if err != nil {
		debugLog("ERROR: No se pudo obtener el directorio del usuario: %v", err)
		home = ""
	}

	shortPath := cwd
	if home != "" && strings.HasPrefix(cwd, home) {
		shortPath = strings.Replace(cwd, home, "~", 1)
	}

	// Acortar path si es muy largo
	if len(shortPath) > 40 {
		pathParts := strings.Split(shortPath, "/")
		if len(pathParts) > 3 {
			shortPath = ".../" + strings.Join(pathParts[len(pathParts)-3:], "/")
		}
	}

	user := os.Getenv("USER")
	if user == "" {
		user = "unknown"
	}

	promptColor := CGreen
	if lastExitCode != "0" {
		promptColor = CRed
	}

	// Print Línea 1
	fmt.Printf("\n%s%s%s %s %s%s%s%s%s%s\n",
		FgMainColor, SepL, BgTop, topContent, Reset, BgTop, Reset, FgMainColor, SepR, Reset)

	// Print Línea 2
	osIcon := getIcon(IconOS)
	if osIcon != "" {
		osIcon += " "
	}
	dirIcon := getIcon(IconDir)
	if dirIcon != "" {
		dirIcon += " "
	}
	fmt.Printf("%s%s%s%s %s%s %s%s%s%s %s%s%s%s%s%s%s%s%s%s\n",
		FgBlueColor, SepL, BgBlue, Black, osIcon, user,
		CBlue, BgMain, SepArrow,
		BgMain, CCyan, dirIcon, White, shortPath,
		Reset, BgMain, Reset, FgMainColor, SepR, Reset)

	// Print Línea 3
	fmt.Printf("%s➜ %s", promptColor, Reset)
}

// ---------------------------------------------------------
//
//	NUEVA LÓGICA DE DETECCIÓN PYTHON (NATIVA)
//
// ---------------------------------------------------------
func getPythonEnv() string {
	cacheKey := "python_env"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("Python: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	// 1. ACTIVOS FÍSICOS (Prioridad Máxima -> VERDE)
	// Detecta virtualenv clásicos y conda activados por terminal
	if venv := os.Getenv("VIRTUAL_ENV"); venv != "" {
		name := filepath.Base(venv)
		icon := getIcon(IconVenv)
		if icon != "" {
			icon += " "
		}
		result := fmt.Sprintf("%s%s%s", CGreen, icon, name)
		saveToCache(cacheKey, result)
		return result
	} else if conda := os.Getenv("CONDA_DEFAULT_ENV"); conda != "" {
		icon := getIcon(IconVenv)
		if icon != "" {
			icon += " "
		}
		result := fmt.Sprintf("%s%s%s", CGreen, icon, conda)
		saveToCache(cacheKey, result)
		return result
	}

	// 2. ACTIVOS VIRTUALES / MANAGERS (Prioridad Media -> AMARILLO)
	// Detecta si Pyenv está activo global o localmente vía variable de entorno
	if pyenv := os.Getenv("PYENV_VERSION"); pyenv != "" {
		icon := getIcon(IconVirt)
		if icon != "" {
			icon += " "
		}
		result := fmt.Sprintf("%s%s%s", CYellow, icon, pyenv)
		saveToCache(cacheKey, result)
		return result
	}

	// 3. BÚSQUEDA RECURSIVA HACIA ARRIBA (Solo Configs)
	// Busca archivos .python-version en carpetas padre
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}

	foundEnv := findPythonContextUpwards(wd)
	if foundEnv != "" {
		saveToCache(cacheKey, foundEnv)
		return foundEnv
	}

	saveToCache(cacheKey, "")
	return ""
}

// findPythonContextUpwards busca entornos físicos (Gris) o versiones definidas (Amarillo)
func findPythonContextUpwards(startDir string) string {
	curr := startDir
	home, _ := os.UserHomeDir()
	root := filepath.VolumeName(startDir) + string(os.PathSeparator)

	// Límite: Máximo subir 6 niveles
	maxLevels := 6

	for i := 0; i < maxLevels; i++ {
		// A. VERSIÓN DEFINIDA (.python-version) -> AMARILLO
		// Esto indica un entorno gestionado por pyenv local
		pyVerPath := filepath.Join(curr, ".python-version")
		if fileExists(pyVerPath) {
			data, err := os.ReadFile(pyVerPath)
			if err == nil {
				ver := strings.TrimSpace(string(data))
				if ver != "" {
					icon := getIcon(IconVirt)
					if icon != "" {
						icon += " "
					}
					return fmt.Sprintf("%s%s%s", CYellow, icon, ver)
				}
			}
		}

		// Condiciones de parada
		if curr == home || curr == root || curr == "." || curr == "/" {
			break
		}
		parent := filepath.Dir(curr)
		if parent == curr {
			break
		}
		curr = parent
	}
	return ""
}

// scanDirForVirtualEnvs busca subcarpetas que sean entornos (Retorna Gris)
func scanDirForVirtualEnvs(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	// Optimización: Ignorar carpetas basura
	ignoreDirs := map[string]bool{
		"node_modules": true, ".git": true, ".idea": true, ".vscode": true,
		"__pycache__": true, "build": true, "dist": true, "target": true,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Ignorar ocultos excepto .venv
		if strings.HasPrefix(name, ".") && name != ".venv" {
			continue
		}
		if ignoreDirs[name] {
			continue
		}

		fullPath := filepath.Join(dir, name)

		// HEURÍSTICA: Verifica si esta carpeta es un entorno
		if isVirtualEnv(fullPath) {
			icon := getIcon(IconVenv)
			if icon != "" {
				icon += " "
			}
			return fmt.Sprintf("%s%s%s (off)", CGrayTxt, icon, name)
		}
	}
	return ""
}

// isVirtualEnv verifica si un directorio tiene estructura de entorno Python
func isVirtualEnv(dirName string) bool {
	// 1. pyvenv.cfg (Rápido)
	if fileExists(filepath.Join(dirName, "pyvenv.cfg")) {
		return true
	}
	// 2. bin/activate (Linux/Mac)
	if fileExists(filepath.Join(dirName, "bin", "activate")) {
		return true
	}
	// 3. Scripts/activate (Windows)
	if fileExists(filepath.Join(dirName, "Scripts", "activate")) {
		return true
	}
	return false
}

// ---------------------------------------------------------
//             OTRAS FUNCIONES (GIT, NODE, K8S, DOCKER, RUST, RUBY, PHP, JAVA)
// ---------------------------------------------------------

func getGitStatus() string {
	cacheKey := "git_status"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("Git: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	// Timeout general para todo el bloque Git
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	// 1. Chequeo rápido (¿Es un repo?)
	// Este lo hacemos síncrono porque si falla, no tiene sentido lanzar lo demás.
	cmdCheck := exec.CommandContext(ctx, "git", "rev-parse", "--is-inside-work-tree")
	cmdCheck.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	if err := cmdCheck.Run(); err != nil {
		saveToCache(cacheKey, "")
		return ""
	}

	// Estructuras para guardar resultados de los hilos
	type gitResult struct {
		Branch      string
		HasConflict bool
		HasStaged   bool
		HasModified bool
		HasUntracked bool
		HasStash    bool
		Ahead       int
		Behind      int
	}
	res := gitResult{}
	
	// Usamos WaitGroup para esperar a los 4 sub-procesos
	var wg sync.WaitGroup
	wg.Add(4)

	// --- HILO 1: RAMA ---
	go func() {
		defer wg.Done()
		cmdBranch := exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "HEAD")
		cmdBranch.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
		out, _ := cmdBranch.Output()
		branch := strings.TrimSpace(string(out))
		
		if branch == "" {
			// Fallback: Tag o Hash
			cmdTag := exec.CommandContext(ctx, "git", "describe", "--tags", "--exact-match")
			cmdTag.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
			outTag, _ := cmdTag.Output()
			branch = strings.TrimSpace(string(outTag))
			if branch == "" {
				cmdHash := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
				cmdHash.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
				outHash, _ := cmdHash.Output()
				branch = ":" + strings.TrimSpace(string(outHash))
			}
		}
		res.Branch = branch
	}()

	// --- HILO 2: STATUS (El más pesado) ---
	go func() {
		defer wg.Done()
		// --porcelain v1 es rápido y fácil de parsear
		cmdStatus := exec.CommandContext(ctx, "git", "status", "--porcelain")
		cmdStatus.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
		out, _ := cmdStatus.Output()
		
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			if len(line) < 2 { continue }
			x := line[0]
			y := line[1]

			if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
				res.HasConflict = true
			} else if x == '?' && y == '?' {
				res.HasUntracked = true
			} else {
				if x != ' ' && x != '?' { res.HasStaged = true }
				if y != ' ' && y != '?' { res.HasModified = true }
			}
		}
	}()

	// --- HILO 3: STASH ---
	go func() {
		defer wg.Done()
		cmdStash := exec.CommandContext(ctx, "git", "rev-list", "--walk-reflogs", "--count", "refs/stash")
		cmdStash.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
		out, _ := cmdStash.Output()
		str := strings.TrimSpace(string(out))
		if str != "" && str != "0" {
			res.HasStash = true
		}
	}()

	// --- HILO 4: REMOTE (Ahead/Behind) ---
	go func() {
		defer wg.Done()
		// Primero verificamos si hay upstream para no lanzar errores
		cmdUp := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "@{u}")
		cmdUp.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
		if err := cmdUp.Run(); err == nil {
			cmdCount := exec.CommandContext(ctx, "git", "rev-list", "--left-right", "--count", "HEAD...@{upstream}")
			cmdCount.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
			out, _ := cmdCount.Output()
			fields := strings.Fields(string(out))
			if len(fields) >= 2 {
				fmt.Sscanf(fields[0], "%d", &res.Ahead)
				fmt.Sscanf(fields[1], "%d", &res.Behind)
			} else if len(fields) == 1 {
				fmt.Sscanf(fields[0], "%d", &res.Ahead)
			}
		}
	}()

	// Esperar a que todos terminen
	wg.Wait()

	// --- CONSTRUIR UI ---
	var color string
	var symbols []string

	if res.HasConflict {
		symbols = append(symbols, "✘")
	}
	if res.Behind > 0 {
		symbols = append(symbols, fmt.Sprintf("%s%d", IconGitPull, res.Behind))
	}
	if res.Ahead > 0 {
		symbols = append(symbols, fmt.Sprintf("%s%d", IconGitPush, res.Ahead))
	}
	if res.HasStaged {
		symbols = append(symbols, IconGitCommit)
	}
	if res.HasModified {
		symbols = append(symbols, "!")
	}
	if res.HasUntracked {
		symbols = append(symbols, "?")
	}
	if res.HasStash {
		symbols = append(symbols, "≡")
	}

	// Lógica de color
	if res.HasConflict {
		color = CRed
	} else if res.Behind > 0 {
		color = CPink
	} else if res.Ahead > 0 {
		color = CPurple
	} else if res.HasStaged {
		color = CLightBlue
	} else if res.HasModified || res.HasUntracked {
		color = COrange
	} else {
		color = CGreen
	}

	gitIcon := getIcon(IconGit)
	if gitIcon != "" { gitIcon += " " }
	
	statusStr := ""
	if len(symbols) > 0 {
		statusStr = " " + strings.Join(symbols, " ")
	}

	result := fmt.Sprintf("%s%s%s%s", color, gitIcon, res.Branch, statusStr)
	saveToCache(cacheKey, result)
	return result
}

func getNodeVersion() string {
	cacheKey := "node_version"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("Node: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	if !fileExists("package.json") {
		debugLog("Node: package.json no encontrado.")
		saveToCache(cacheKey, "")
		return ""
	}

	data, err := os.ReadFile("package.json")
	if err != nil {
		debugLog("ERROR: os.ReadFile(\"package.json\") falló en getNodeVersion: %v", err)
		saveToCache(cacheKey, "") // Caché resultado vacío en caso de error de lectura
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.Contains(line, `"version"`) {
			parts := strings.Split(line, ":")
			if len(parts) >= 2 {
				ver := strings.Trim(parts[1], ` ",`)
				nodeIcon := getIcon(IconNode)
				if nodeIcon != "" {
					nodeIcon += " "
				}
				result := fmt.Sprintf("%s%s%s", CGreen, nodeIcon, ver)
				saveToCache(cacheKey, result)
				return result
			}
		}
	}
	// Si no se encuentra "version" pero hay package.json, asumimos un proyecto Node genérico
	nodeIcon := getIcon(IconNode)
	if nodeIcon != "" {
		nodeIcon += " "
	}
	result := fmt.Sprintf("%s%sProj", CGreen, nodeIcon)
	saveToCache(cacheKey, result)
	return result
}

func getK8sContext() string {
	cacheKey := "k8s_context"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("K8s: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	home, err := os.UserHomeDir()
	if err != nil {
		debugLog("ERROR: os.UserHomeDir() falló en getK8sContext: %v", err)
		saveToCache(cacheKey, "")
		return ""
	}

	// Chequeo de archivo primero para evitar lag de kubectl
	kubeConfig := filepath.Join(home, ".kube", "config")
	if !fileExists(kubeConfig) && os.Getenv("KUBECONFIG") == "" {
		debugLog("K8s: No se encontró .kube/config ni KUBECONFIG.")
		saveToCache(cacheKey, "")
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "kubectl", "config", "current-context").Output()
	if err != nil {
		debugLog("ERROR: kubectl config current-context falló o timeout: %v", err)
		saveToCache(cacheKey, "")
		return ""
	}

	ctxName := strings.TrimSpace(string(out))
	if ctxName == "" {
		debugLog("K8s: Contexto de kubectl vacío.")
		saveToCache(cacheKey, "")
		return ""
	}
	k8sIcon := getIcon(IconK8s)
	if k8sIcon != "" {
		k8sIcon += " "
	}
	result := fmt.Sprintf("%s%s%s", CCyan, k8sIcon, ctxName)
	saveToCache(cacheKey, result)
	return result
}

func getDockerStatus() string {
	cacheKey := "docker_status"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("Docker: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	// Detección por archivo, 1000x más rápido que 'docker ps'
	dockerFiles := []string{"Dockerfile", "docker-compose.yml", "docker-compose.yaml", ".dockerignore"}

	for _, file := range dockerFiles {
		if fileExists(file) {
			debugLog("Docker: Archivo %s encontrado.", file)
			dockerIcon := getIcon(IconDocker)
			result := fmt.Sprintf("%s%s", CBlue, dockerIcon)
			saveToCache(cacheKey, result)
			return result
		}
	}
	saveToCache(cacheKey, "")
	return ""
}

func getRustStatus() string {
	cacheKey := "rust_status"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("Rust: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	if fileExists("Cargo.toml") {
		debugLog("Rust: Cargo.toml encontrado.")
		rustIcon := getIcon(IconRust)
		result := fmt.Sprintf("%s%s", CYellow, rustIcon) // Usamos amarillo para proyectos, verde si hay una versión específica
		// Intentar obtener la versión si es relevante y no impacta mucho en el rendimiento
		// Por ahora, solo detectamos el proyecto.
		saveToCache(cacheKey, result)
		return result
	}
	saveToCache(cacheKey, "")
	return ""
}

func getRubyStatus() string {
	cacheKey := "ruby_status"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("Ruby: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	if fileExists("Gemfile") || fileExists("Rakefile") {
		debugLog("Ruby: Gemfile o Rakefile encontrado.")
		rubyIcon := getIcon(IconRuby)
		result := fmt.Sprintf("%s%s", CRed, rubyIcon) // Rojo para Ruby, por ejemplo
		// Podríamos intentar obtener la versión de Ruby o de Bundler
		saveToCache(cacheKey, result)
		return result
	}
	saveToCache(cacheKey, "")
	return ""
}

func getPhpStatus() string {
	cacheKey := "php_status"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("PHP: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	if fileExists("composer.json") || fileExists("artisan") {
		debugLog("PHP: composer.json o artisan encontrado.")
		phpIcon := getIcon(IconPHP)
		result := fmt.Sprintf("%s%s", CBlue, phpIcon) // Azul para PHP
		// Podríamos intentar obtener la versión de PHP o de Laravel
		saveToCache(cacheKey, result)
		return result
	}
	saveToCache(cacheKey, "")
	return ""
}

func getJavaStatus() string {
	cacheKey := "java_status"
	if cachedVal := getFromCache(cacheKey); cachedVal != "" {
		debugLog("Java: Usando valor cacheado: %s", cachedVal)
		return cachedVal
	}

	if fileExists("pom.xml") || fileExists("build.gradle") || fileExists("src/main/java") {
		debugLog("Java: pom.xml, build.gradle o src/main/java encontrado.")
		javaIcon := getIcon(IconJava)
		result := fmt.Sprintf("%s%s", CGreen, javaIcon) // Verde para Java
		// Podríamos intentar obtener la versión de Java o del proyecto
		saveToCache(cacheKey, result)
		return result
	}
	saveToCache(cacheKey, "")
	return ""
}

// ---------------------------------------------------------
//                    UTILIDADES (CACHÉ, DEBUG, FILE EXISTS)
// ---------------------------------------------------------

// initAppConfig inicializa la configuración de la aplicación
func initAppConfig(noIcons, installCmd bool, shellType string) {
	AppConfig.DebugMode = os.Getenv("PROMPT_DEBUG") == "1"
	AppConfig.NoIcons = noIcons || os.Getenv("PROMPT_NO_ICONS") == "1"
	AppConfig.InstallCmd = installCmd

	// Detectar shell
	if shellType != "" {
		AppConfig.ShellType = strings.ToLower(shellType)
	} else {
		AppConfig.ShellType = detectShell()
	}

	debugLog("Configuración inicializada - Debug: %t, NoIcons: %t, Shell: %s",
		AppConfig.DebugMode, AppConfig.NoIcons, AppConfig.ShellType)
}

// detectShell detecta la shell actual ($SHELL)
func detectShell() string {
	shell := os.Getenv("SHELL")
	debugLog("$SHELL = %s", shell)

	if strings.Contains(shell, "zsh") {
		return "zsh"
	} else if strings.Contains(shell, "bash") {
		return "bash"
	} else if strings.Contains(shell, "fish") {
		return "fish"
	}

	return "other"
}

// wrapColor envuelve un código ANSI según la shell
func wrapColor(code string) string {
	switch AppConfig.ShellType {
	case "bash":
		return "\\[" + code + "\\]"
	case "zsh":
		return "%{" + code + "%}"
	case "fish":
		// Fish no necesita wrapping especial, usa ANSI directo
		return code
	default:
		return code // Sin envoltura para otras shells
	}
}

// initColors inicializa las variables de color según la shell detectada
func initColors() {
	Reset = wrapColor(ResetCode)
	White = wrapColor(WhiteCode)
	Black = wrapColor(BlackCode)
	CBlue = wrapColor(CBlueCode)
	CCyan = wrapColor(CCyanCode)
	CGreen = wrapColor(CGreenCode)
	CYellow = wrapColor(CYellowCode)
	COrange = wrapColor(COrangeCode)
	CPurple = wrapColor(CPurpleCode)
	CLightBlue = wrapColor(CLightBlueCode)
	CPink = wrapColor(CPinkCode)
	CRed = wrapColor(CRedCode)
	CGrayTxt = wrapColor(CGrayTxtCode)
	BgMain = wrapColor(BgMainCode)
	BgTop = wrapColor(BgTopCode)
	BgBlue = wrapColor(BgBlueCode)
	FgMainColor = wrapColor(FgMainColorCode)
	FgBlueColor = wrapColor(FgBlueColorCode)

	debugLog("Colores inicializados para shell: %s", AppConfig.ShellType)
}

// getIcon retorna un icono o vacío según configuración
func getIcon(icon string) string {
	if AppConfig.NoIcons {
		return ""
	}
	return icon
}

// generateInstallScript genera un script de instalación para configurar el prompt
func generateInstallScript() {
	binPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: No se pudo obtener la ruta del ejecutable: %v\n", err)
		os.Exit(1)
	}

	shell := detectShell()

	fmt.Println("# ================================================")
	fmt.Println("# Script de Instalación del Prompt Personalizado")
	fmt.Println("# ================================================")
	fmt.Println()
	fmt.Printf("# Shell detectada: %s\n", shell)
	fmt.Println()

	switch shell {
	case "bash":
		fmt.Println("# Instrucciones para Bash:")
		fmt.Println("# 1. Agregar las siguientes líneas a tu ~/.bashrc:")
		fmt.Println()
		fmt.Println("# Función para prompt personalizado")
		fmt.Println("function __custom_prompt() {")
		fmt.Printf("    PS1=\"$(%s $?)\"", binPath)
		fmt.Println()
		fmt.Println("}")
		fmt.Println()
		fmt.Println("# Configurar PROMPT_COMMAND")
		fmt.Println("PROMPT_COMMAND=__custom_prompt")
		fmt.Println()
		fmt.Println("# 2. Recargar la configuración:")
		fmt.Println("#    source ~/.bashrc")
		fmt.Println()
		fmt.Println("# Opcional: Deshabilitar iconos si no tienes Nerd Fonts:")
		fmt.Println("#    export PROMPT_NO_ICONS=1")

	case "zsh":
		fmt.Println("# Instrucciones para Zsh:")
		fmt.Println("# 1. Agregar las siguientes líneas a tu ~/.zshrc:")
		fmt.Println()
		fmt.Println("# Función para prompt personalizado")
		fmt.Println("function precmd() {")
		fmt.Printf("    PROMPT=\"$(%s --shell=zsh $?)\"", binPath)
		fmt.Println()
		fmt.Println("}")
		fmt.Println()
		fmt.Println("# Habilitar expansión de prompt")
		fmt.Println("setopt PROMPT_SUBST")
		fmt.Println()
		fmt.Println("# 2. Recargar la configuración:")
		fmt.Println("#    source ~/.zshrc")
		fmt.Println()
		fmt.Println("# Opcional: Deshabilitar iconos si no tienes Nerd Fonts:")
		fmt.Println("#    export PROMPT_NO_ICONS=1")

	case "fish":
		fmt.Println("# Instrucciones para Fish Shell:")
		fmt.Println("# 1. Agregar las siguientes líneas a tu ~/.config/fish/config.fish:")
		fmt.Println()
		fmt.Println("# Función para prompt personalizado")
		fmt.Println("function fish_prompt")
		fmt.Printf("    %s --shell=fish $status", binPath)
		fmt.Println()
		fmt.Println("end")
		fmt.Println()
		fmt.Println("# 2. Recargar la configuración:")
		fmt.Println("#    source ~/.config/fish/config.fish")
		fmt.Println()
		fmt.Println("# Opcional: Deshabilitar iconos si no tienes Nerd Fonts:")
		fmt.Println("#    set -x PROMPT_NO_ICONS 1")

	default:
		fmt.Println("# Shell no reconocida automáticamente.")
		fmt.Println("# Por favor, consulta la documentación de tu shell para agregar:")
		fmt.Printf("#    PS1=\"$(%s $?)\"\n", binPath)
	}

	fmt.Println()
	fmt.Println("# ================================================")
	fmt.Println("# Opciones adicionales:")
	fmt.Println("# ================================================")
	fmt.Println("# - Modo debug: export PROMPT_DEBUG=1")
	fmt.Println("# - Sin iconos:  export PROMPT_NO_ICONS=1")
	fmt.Println("# - Forzar shell: --shell=bash|zsh|fish")
	fmt.Println()
}

// debugLog imprime un mensaje si el modo debug está activado
func debugLog(format string, a ...interface{}) {
	if AppConfig.DebugMode {
		fmt.Fprintf(os.Stderr, "[DEBUG] "+format+"\n", a...)
	}
}

// fileExists verifica si un archivo o directorio existe
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// loadDirectoryCache carga el listado de archivos del directorio actual en caché
func loadDirectoryCache() {
	directoryCache.Lock()
	defer directoryCache.Unlock()

	// Si el caché es válido, no recargar
	if time.Now().Before(directoryCache.expiration) {
		debugLog("DirectoryCache: Usando caché válido")
		return
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		debugLog("ERROR: os.ReadDir(\".\") falló en loadDirectoryCache: %v", err)
		return
	}

	files := make([]string, 0)
	dirs := make([]string, 0)

	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, entry.Name())
		} else {
			files = append(files, entry.Name())
		}
	}

	directoryCache.files = files
	directoryCache.dirs = dirs
	directoryCache.expiration = time.Now().Add(cacheTTL)

	debugLog("DirectoryCache: Cargado %d archivos, %d directorios", len(files), len(dirs))
}

// hasFileWithExtension verifica si existe algún archivo con la extensión especificada
func hasFileWithExtension(ext string) bool {
	directoryCache.RLock()
	defer directoryCache.RUnlock()

	for _, file := range directoryCache.files {
		if strings.HasSuffix(file, ext) {
			debugLog("hasFileWithExtension: Encontrado %s", file)
			return true
		}
	}
	return false
}

// hasAnyFile verifica si existe alguno de los archivos especificados
func hasAnyFile(files []string) bool {
	directoryCache.RLock()
	defer directoryCache.RUnlock()

	fileSet := make(map[string]bool)
	for _, f := range directoryCache.files {
		fileSet[f] = true
	}

	for _, file := range files {
		if fileSet[file] {
			debugLog("hasAnyFile: Encontrado %s", file)
			return true
		}
	}
	return false
}

// getCachedDirs retorna los directorios cacheados
func getCachedDirs() []string {
	directoryCache.RLock()
	defer directoryCache.RUnlock()

	// Retornar copia para evitar race conditions
	dirs := make([]string, len(directoryCache.dirs))
	copy(dirs, directoryCache.dirs)
	return dirs
}

// hasPythonIndicators verifica si hay indicios de Python en el directorio
func hasPythonIndicators() bool {
	// Chequeos super rápidos primero
	if os.Getenv("VIRTUAL_ENV") != "" ||
		os.Getenv("CONDA_DEFAULT_ENV") != "" ||
		os.Getenv("PYENV_VERSION") != "" {
		debugLog("hasPythonIndicators: Variables de entorno detectadas")
		return true
	}

	// Archivos Python comunes
	pythonFiles := []string{
		".python-version",
		"requirements.txt",
		"setup.py",
		"pyproject.toml",
		"poetry.lock",
		"Pipfile",
		"manage.py", // Django
		"app.py",    // Flask común
		"main.py",
		"pyvenv.cfg",
	}

	if hasAnyFile(pythonFiles) {
		debugLog("hasPythonIndicators: Archivo de configuración Python encontrado")
		return true
	}

	// Archivos .py
	if hasFileWithExtension(".py") {
		debugLog("hasPythonIndicators: Archivos .py encontrados")
		return true
	}

	debugLog("hasPythonIndicators: No se encontraron indicios de Python")
	return false
}

// getFromCache intenta recuperar un valor de la caché
func getFromCache(key string) string {
	globalCache.RLock()
	defer globalCache.RUnlock()

	entry, found := globalCache.data[key]
	if !found {
		debugLog("CACHE: '%s' no encontrada.", key)
		return ""
	}

	if time.Now().Before(entry.Expiration) {
		debugLog("CACHE: '%s' encontrada y válida.", key)
		return entry.Value
	}

	debugLog("CACHE: '%s' encontrada pero expirada.", key)
	return "" // Expirado
}

// saveToCache guarda un valor en la caché con una expiración
func saveToCache(key, value string) {
	globalCache.Lock()
	defer globalCache.Unlock()

	globalCache.data[key] = CacheEntry{
		Value:      value,
		Expiration: time.Now().Add(cacheTTL),
	}
	debugLog("CACHE: '%s' guardada con valor '%s', expira en %s", key, value, cacheTTL)
}
