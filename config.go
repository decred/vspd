package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/decred/dcrd/dcrutil/v3"
	flags "github.com/jessevdk/go-flags"
)

var (
	defaultListen         = ":3000"
	defaultLogLevel       = "debug"
	defaultVSPFee         = 0.01
	defaultNetwork        = "testnet"
	defaultHomeDir        = dcrutil.AppDataDir("dcrvsp", false)
	defaultConfigFilename = "dcrvsp.conf"
	defaultConfigFile     = filepath.Join(defaultHomeDir, defaultConfigFilename)
	defaultWalletHost     = "127.0.0.1"
)

// config defines the configuration options for the VSP.
type config struct {
	Listen     string  `long:"listen" ini-name:"listen" description:"The ip:port to listen for API requests."`
	LogLevel   string  `long:"loglevel" ini-name:"loglevel" description:"Logging level." choice:"trace" choice:"debug" choice:"info" choice:"warn" choice:"error" choice:"critical"`
	Network    string  `long:"network" ini-name:"network" description:"Decred network to use." choice:"testnet" choice:"mainnet" choice:"simnet"`
	VSPFee     float64 `long:"vspfee" ini-name:"vspfee" description:"The fee percentage charged for VSP use. eg. 0.01 (1%), 0.05 (5%)."`
	HomeDir    string  `long:"homedir" ini-name:"homedir" no-ini:"true" description:"Path to application home directory. Used for storing VSP database and logs."`
	ConfigFile string  `long:"configfile" ini-name:"configfile" no-ini:"true" description:"Path to configuration file."`
	WalletHost string  `long:"wallethost" ini-name:"wallethost" description:"The ip:port to establish a JSON-RPC connection with dcrwallet."`
	WalletUser string  `long:"walletuser" ini-name:"walletuser" description:"Username for dcrwallet RPC connections."`
	WalletPass string  `long:"walletpass" ini-name:"walletpass" description:"Password for dcrwallet RPC connections."`
	WalletCert string  `long:"walletcert" ini-name:"walletcert" description:"The dcrwallet RPC certificate file."`

	signKey   ed25519.PrivateKey
	pubKey    ed25519.PublicKey
	dbPath    string
	netParams *netParams
	dcrwCert  []byte
}

// fileExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); os.IsNotExist(err) {
		return false
	}
	return true
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Nothing to do when no path is given.
	if path == "" {
		return path
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows cmd.exe-style
	// %VARIABLE%, but the variables can still be expanded via POSIX-style
	// $VARIABLE.
	path = os.ExpandEnv(path)

	if !strings.HasPrefix(path, "~") {
		return filepath.Clean(path)
	}

	// Expand initial ~ to the current user's home directory, or ~otheruser
	// to otheruser's home directory.  On Windows, both forward and backward
	// slashes can be used.
	path = path[1:]

	var pathSeparators string
	if runtime.GOOS == "windows" {
		pathSeparators = string(os.PathSeparator) + "/"
	} else {
		pathSeparators = string(os.PathSeparator)
	}

	userName := ""
	if i := strings.IndexAny(path, pathSeparators); i != -1 {
		userName = path[:i]
		path = path[i:]
	}

	homeDir := ""
	var u *user.User
	var err error
	if userName == "" {
		u, err = user.Current()
	} else {
		u, err = user.Lookup(userName)
	}
	if err == nil {
		homeDir = u.HomeDir
	}
	// Fallback to CWD if user lookup fails or user has no home directory.
	if homeDir == "" {
		homeDir = "."
	}

	return filepath.Join(homeDir, path)
}

// normalizeAddress returns addr with the passed default port appended if
// there is not already a port specified.
func normalizeAddress(addr, defaultPort string) string {
	_, _, err := net.SplitHostPort(addr)
	if err != nil {
		return net.JoinHostPort(addr, defaultPort)
	}
	return addr
}

// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
// 	1) Start with a default config with sane settings
// 	2) Pre-parse the command line to check for an alternative config file
// 	3) Load configuration file overwriting defaults with any specified options
// 	4) Parse CLI options and overwrite/add any specified options
//
// The above results in dcrvsp functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, error) {

	// Default config.
	cfg := config{
		Listen:     defaultListen,
		LogLevel:   defaultLogLevel,
		Network:    defaultNetwork,
		VSPFee:     defaultVSPFee,
		HomeDir:    defaultHomeDir,
		ConfigFile: defaultConfigFile,
		WalletHost: defaultWalletHost,
	}

	// Pre-parse the command line options to see if an alternative config
	// file or the version flag was specified.  Any errors aside from the
	// help message error can be ignored here since they will be caught by
	// the final parse below.
	preCfg := cfg

	preParser := flags.NewParser(&preCfg, flags.HelpFlag)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else if ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stdout, err)
			os.Exit(0)
		}
	}

	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)

	// Update the home directory if specified on CLI. Since the home
	// directory is updated, other variables need to be updated to
	// reflect the new changes.
	if preCfg.HomeDir != "" {
		cfg.HomeDir = cleanAndExpandPath(cfg.HomeDir)
		cfg.HomeDir, _ = filepath.Abs(preCfg.HomeDir)

		if preCfg.ConfigFile == defaultConfigFile {
			defaultConfigFile = filepath.Join(cfg.HomeDir, defaultConfigFilename)
			preCfg.ConfigFile = defaultConfigFile
			cfg.ConfigFile = defaultConfigFile
		} else {
			cfg.ConfigFile = preCfg.ConfigFile
		}
	}

	// Create the home directory if it doesn't already exist.
	funcName := "loadConfig"
	err = os.MkdirAll(cfg.HomeDir, 0700)
	if err != nil {
		str := "%s: failed to create home directory: %v"
		err := fmt.Errorf(str, funcName, err)
		fmt.Fprintln(os.Stderr, err)
		return nil, err
	}

	// Create a default config file when one does not exist and the user did
	// not specify an override.
	if preCfg.ConfigFile == defaultConfigFile && !fileExists(preCfg.ConfigFile) {
		preIni := flags.NewIniParser(preParser)
		err = preIni.WriteFile(preCfg.ConfigFile,
			flags.IniIncludeComments|flags.IniIncludeDefaults)
		if err != nil {
			return nil, fmt.Errorf("error creating a default "+
				"config file: %v", err)
		}
	}

	// Load additional config from file.
	parser := flags.NewParser(&cfg, flags.Default)

	err = flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing config file: %v\n", err)
		os.Exit(1)
	}

	// Parse command line options again to ensure they take precedence.
	_, err = parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, usageMessage)
		}
		return nil, err
	}

	// Set the active network.
	switch cfg.Network {
	case "testnet":
		cfg.netParams = &testNet3Params
	case "mainnet":
		cfg.netParams = &mainNetParams
	case "simnet":
		cfg.netParams = &simNetParams
	}

	// Ensure the dcrwallet RPC username is set.
	if cfg.WalletUser == "" {
		str := "%s: the walletuser option is not set"
		err := fmt.Errorf(str, funcName)
		return nil, err
	}

	// Ensure the dcrwallet RPC password is set.
	if cfg.WalletPass == "" {
		str := "%s: the walletpass option is not set"
		err := fmt.Errorf(str, funcName)
		return nil, err
	}

	// Ensure the dcrwallet RPC cert path is set.
	if cfg.WalletCert == "" {
		str := "%s: the walletcert option is not set"
		err := fmt.Errorf(str, funcName)
		return nil, err
	}

	// Add default port for the active network if there is no port specified.
	cfg.WalletHost = normalizeAddress(cfg.WalletHost, cfg.netParams.WalletRPCServerPort)

	// Load dcrwallet RPC certificate.
	cfg.WalletCert = cleanAndExpandPath(cfg.WalletCert)
	cfg.dcrwCert, err = ioutil.ReadFile(cfg.WalletCert)
	if err != nil {
		str := "%s: failed to read dcrwallet cert file: %s"
		err := fmt.Errorf(str, funcName, err)
		return nil, err
	}

	// Create the data directory.
	dataDir := filepath.Join(cfg.HomeDir, "data", cfg.netParams.Name)
	err = os.MkdirAll(dataDir, 0700)
	if err != nil {
		str := "%s: failed to create data directory: %v"
		err := fmt.Errorf(str, funcName, err)
		return nil, err
	}

	// Initialize loggers and log rotation.
	logDir := filepath.Join(cfg.HomeDir, "logs", cfg.netParams.Name)
	initLogRotator(filepath.Join(logDir, "dcrvsp.log"))
	setLogLevels(cfg.LogLevel)

	// Set the database path
	cfg.dbPath = filepath.Join(dataDir, "vsp.db")

	// Set pubKey/signKey. Read from seed file if it exists, otherwise generate
	// one.
	seedPath := filepath.Join(cfg.HomeDir, "sign.seed")
	seed, err := ioutil.ReadFile(seedPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, errors.New("seedPath does not exist")
		}

		_, cfg.signKey, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate signing key: %v", err)
		}
		err = ioutil.WriteFile(seedPath, cfg.signKey.Seed(), 0400)
		if err != nil {
			return nil, fmt.Errorf("failed to save signing key: %v", err)
		}
	} else {
		cfg.signKey = ed25519.NewKeyFromSeed(seed)
	}

	// Derive pubKey from signKey
	pubKey, ok := cfg.signKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("failed to cast signing key: %T", pubKey)
	}
	cfg.pubKey = pubKey

	return &cfg, nil
}
