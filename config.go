package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"decred.org/dcrwallet/wallet/txrules"
	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/hdkeychain/v3"
	flags "github.com/jessevdk/go-flags"
)

var (
	defaultListen         = ":3000"
	defaultLogLevel       = "debug"
	defaultVSPFee         = 5.0
	defaultNetwork        = "testnet"
	defaultHomeDir        = dcrutil.AppDataDir("vspd", false)
	defaultConfigFilename = "vspd.conf"
	defaultConfigFile     = filepath.Join(defaultHomeDir, defaultConfigFilename)
	defaultDcrdHost       = "127.0.0.1"
	defaultWalletHost     = "127.0.0.1"
	defaultWebServerDebug = false
	defaultBackupInterval = time.Minute * 3
	defaultVspClosed      = false
)

// config defines the configuration options for the VSP.
type config struct {
	Listen         string        `long:"listen" ini-name:"listen" description:"The ip:port to listen for API requests."`
	LogLevel       string        `long:"loglevel" ini-name:"loglevel" description:"Logging level." choice:"trace" choice:"debug" choice:"info" choice:"warn" choice:"error" choice:"critical"`
	Network        string        `long:"network" ini-name:"network" description:"Decred network to use." choice:"testnet" choice:"mainnet" choice:"simnet"`
	FeeXPub        string        `long:"feexpub" ini-name:"feexpub" description:"Cold wallet xpub used for collecting fees."`
	VSPFee         float64       `long:"vspfee" ini-name:"vspfee" description:"Fee percentage charged for VSP use. eg. 2.0 (2%), 0.5 (0.5%)."`
	HomeDir        string        `long:"homedir" ini-name:"homedir" no-ini:"true" description:"Path to application home directory. Used for storing VSP database and logs."`
	ConfigFile     string        `long:"configfile" ini-name:"configfile" no-ini:"true" description:"Path to configuration file."`
	DcrdHost       string        `long:"dcrdhost" ini-name:"dcrdhost" description:"The ip:port to establish a JSON-RPC connection with dcrd. Should be the same host where vspd is running."`
	DcrdUser       string        `long:"dcrduser" ini-name:"dcrduser" description:"Username for dcrd RPC connections."`
	DcrdPass       string        `long:"dcrdpass" ini-name:"dcrdpass" description:"Password for dcrd RPC connections."`
	DcrdCert       string        `long:"dcrdcert" ini-name:"dcrdcert" description:"The dcrd RPC certificate file."`
	WalletHosts    []string      `long:"wallethost" ini-name:"wallethost" description:"Add an ip:port to establish a JSON-RPC connection with voting dcrwallet."`
	WalletUser     string        `long:"walletuser" ini-name:"walletuser" description:"Username for dcrwallet RPC connections."`
	WalletPass     string        `long:"walletpass" ini-name:"walletpass" description:"Password for dcrwallet RPC connections."`
	WalletCert     string        `long:"walletcert" ini-name:"walletcert" description:"The dcrwallet RPC certificate file."`
	WebServerDebug bool          `long:"webserverdebug" ini-name:"webserverdebug" description:"Enable web server debug mode (verbose logging to terminal and live-reloading templates)."`
	SupportEmail   string        `long:"supportemail" ini-name:"supportemail" description:"Email address for users in need of support."`
	BackupInterval time.Duration `long:"backupinterval" ini-name:"backupinterval" description:"Time period between automatic database backups. Valid time units are {s,m,h}. Minimum 30 seconds."`
	VspClosed      bool          `long:"vspclosed" ini-name:"vspclosed" description:"Closed prevents the VSP from accepting new tickets."`

	dbPath     string
	netParams  *netParams
	dcrdCert   []byte
	walletCert []byte
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
// The above results in vspd functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, error) {

	// Default config.
	cfg := config{
		Listen:         defaultListen,
		LogLevel:       defaultLogLevel,
		Network:        defaultNetwork,
		VSPFee:         defaultVSPFee,
		HomeDir:        defaultHomeDir,
		ConfigFile:     defaultConfigFile,
		DcrdHost:       defaultDcrdHost,
		WalletHosts:    []string{defaultWalletHost},
		WebServerDebug: defaultWebServerDebug,
		BackupInterval: defaultBackupInterval,
		VspClosed:      defaultVspClosed,
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
	err = os.MkdirAll(cfg.HomeDir, 0700)
	if err != nil {
		err := fmt.Errorf("failed to create home directory: %v", err)
		fmt.Fprintln(os.Stderr, err)
		return nil, err
	}

	// Create a default config file when one does not exist and the user did
	// not specify an override.
	if preCfg.ConfigFile == defaultConfigFile && !fileExists(preCfg.ConfigFile) {
		fmt.Printf("Writing a config file with default values to %s\n", defaultConfigFile)
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
	minRequired := 1
	switch cfg.Network {
	case "testnet":
		cfg.netParams = &testNet3Params
	case "mainnet":
		cfg.netParams = &mainNetParams
		minRequired = 3
	case "simnet":
		cfg.netParams = &simNetParams
	}

	// Ensure backup interval is greater than 30 seconds.
	if cfg.BackupInterval < time.Second*30 {
		return nil, errors.New("minimum backupinterval is 30 seconds")
	}

	// Ensure the fee percentage is valid per txrules.
	if !txrules.ValidPoolFeeRate(cfg.VSPFee) {
		return nil, errors.New("invalid vspfee - should be greater than 0.01 and less than 100.0 ")
	}

	// Ensure the support email address is set.
	if cfg.SupportEmail == "" {
		return nil, errors.New("the supportemail option is not set")
	}

	// Ensure the dcrd RPC username is set.
	if cfg.DcrdUser == "" {
		return nil, errors.New("the dcrduser option is not set")
	}

	// Ensure the dcrd RPC password is set.
	if cfg.DcrdPass == "" {
		return nil, errors.New("the dcrdpass option is not set")
	}

	// Ensure the dcrd RPC cert path is set.
	if cfg.DcrdCert == "" {
		return nil, errors.New("the dcrdcert option is not set")
	}

	// Load dcrd RPC certificate.
	cfg.DcrdCert = cleanAndExpandPath(cfg.DcrdCert)
	cfg.dcrdCert, err = ioutil.ReadFile(cfg.DcrdCert)
	if err != nil {
		return nil, fmt.Errorf("failed to read dcrd cert file: %v", err)
	}

	// Ensure the dcrwallet RPC username is set.
	if cfg.WalletUser == "" {
		return nil, errors.New("the walletuser option is not set")
	}

	// Ensure the dcrwallet RPC password is set.
	if cfg.WalletPass == "" {
		return nil, errors.New("the walletpass option is not set")
	}

	// Ensure the dcrwallet RPC cert path is set.
	if cfg.WalletCert == "" {
		return nil, errors.New("the walletcert option is not set")
	}

	// Load dcrwallet RPC certificate.
	cfg.WalletCert = cleanAndExpandPath(cfg.WalletCert)
	cfg.walletCert, err = ioutil.ReadFile(cfg.WalletCert)
	if err != nil {
		return nil, fmt.Errorf("failed to read dcrwallet cert file: %v", err)
	}

	// Verify minimum number of voting wallets are configured.
	if len(cfg.WalletHosts) < minRequired {
		return nil, fmt.Errorf("minimum required voting wallets has not been met: %d < %d",
			len(cfg.WalletHosts), minRequired)
	}

	// Add default port for the active network if there is no port specified.
	for i := 0; i < len(cfg.WalletHosts); i++ {
		cfg.WalletHosts[i] = normalizeAddress(cfg.WalletHosts[i], cfg.netParams.WalletRPCServerPort)
	}
	cfg.DcrdHost = normalizeAddress(cfg.DcrdHost, cfg.netParams.DcrdRPCServerPort)

	// Create the data directory.
	dataDir := filepath.Join(cfg.HomeDir, "data", cfg.netParams.Name)
	err = os.MkdirAll(dataDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("failed to create data directory: %v", err)
	}

	// Initialize loggers and log rotation.
	logDir := filepath.Join(cfg.HomeDir, "logs", cfg.netParams.Name)
	initLogRotator(filepath.Join(logDir, "vspd.log"))
	setLogLevels(cfg.LogLevel)

	// Set the database path
	cfg.dbPath = filepath.Join(dataDir, "vspd.db")

	// Validate the cold wallet xpub.
	if cfg.FeeXPub == "" {
		return nil, errors.New("the feexpub option is not set")
	}
	_, err = hdkeychain.NewKeyFromString(cfg.FeeXPub, cfg.netParams.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to parse feexpub: %v", err)
	}

	return &cfg, nil
}
