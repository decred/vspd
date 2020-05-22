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

	"github.com/decred/dcrd/dcrutil/v3"
	"github.com/decred/dcrd/hdkeychain/v3"
	flags "github.com/jessevdk/go-flags"
)

var (
	defaultListen           = ":3000"
	defaultLogLevel         = "debug"
	defaultVSPFee           = 0.01
	defaultNetwork          = "testnet"
	defaultHomeDir          = dcrutil.AppDataDir("dcrvsp", false)
	defaultConfigFilename   = "dcrvsp.conf"
	defaultConfigFile       = filepath.Join(defaultHomeDir, defaultConfigFilename)
	defaultFeeWalletHost    = "127.0.0.1"
	defaultVotingWalletHost = "127.0.0.1"
	defaultWebServerDebug   = false
)

// config defines the configuration options for the VSP.
type config struct {
	Listen           string  `long:"listen" ini-name:"listen" description:"The ip:port to listen for API requests."`
	LogLevel         string  `long:"loglevel" ini-name:"loglevel" description:"Logging level." choice:"trace" choice:"debug" choice:"info" choice:"warn" choice:"error" choice:"critical"`
	Network          string  `long:"network" ini-name:"network" description:"Decred network to use." choice:"testnet" choice:"mainnet" choice:"simnet"`
	FeeXPub          string  `long:"feexpub" ini-name:"feexpub" description:"Cold wallet xpub used for collecting fees."`
	VSPFee           float64 `long:"vspfee" ini-name:"vspfee" description:"Fee percentage charged for VSP use. eg. 0.01 (1%), 0.05 (5%)."`
	HomeDir          string  `long:"homedir" ini-name:"homedir" no-ini:"true" description:"Path to application home directory. Used for storing VSP database and logs."`
	ConfigFile       string  `long:"configfile" ini-name:"configfile" no-ini:"true" description:"Path to configuration file."`
	FeeWalletHost    string  `long:"feewallethost" ini-name:"feewallethost" description:"The ip:port to establish a JSON-RPC connection with fee dcrwallet."`
	FeeWalletUser    string  `long:"feewalletuser" ini-name:"feewalletuser" description:"Username for fee dcrwallet RPC connections."`
	FeeWalletPass    string  `long:"feewalletpass" ini-name:"feewalletpass" description:"Password for fee dcrwallet RPC connections."`
	FeeWalletCert    string  `long:"feewalletcert" ini-name:"feewalletcert" description:"The fee dcrwallet RPC certificate file."`
	VotingWalletHost string  `long:"votingwallethost" ini-name:"votingwallethost" description:"The ip:port to establish a JSON-RPC connection with voting dcrwallet."`
	VotingWalletUser string  `long:"votingwalletuser" ini-name:"votingwalletuser" description:"Username for voting dcrwallet RPC connections."`
	VotingWalletPass string  `long:"votingwalletpass" ini-name:"votingwalletpass" description:"Password for voting dcrwallet RPC connections."`
	VotingWalletCert string  `long:"votingwalletcert" ini-name:"votingwalletcert" description:"The voting dcrwallet RPC certificate file."`
	WebServerDebug   bool    `long:"webserverdebug" ini-name:"webserverdebug" description:"Enable web server debug mode (verbose logging to terminal and live-reloading templates)."`

	dbPath           string
	netParams        *netParams
	feeWalletCert    []byte
	votingWalletCert []byte
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
		Listen:           defaultListen,
		LogLevel:         defaultLogLevel,
		Network:          defaultNetwork,
		VSPFee:           defaultVSPFee,
		HomeDir:          defaultHomeDir,
		ConfigFile:       defaultConfigFile,
		FeeWalletHost:    defaultFeeWalletHost,
		VotingWalletHost: defaultVotingWalletHost,
		WebServerDebug:   defaultWebServerDebug,
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
	switch cfg.Network {
	case "testnet":
		cfg.netParams = &testNet3Params
	case "mainnet":
		cfg.netParams = &mainNetParams
	case "simnet":
		cfg.netParams = &simNetParams
	}

	// Ensure the fee dcrwallet RPC username is set.
	if cfg.FeeWalletUser == "" {
		return nil, errors.New("the feewalletuser option is not set")
	}

	// Ensure the fee dcrwallet RPC password is set.
	if cfg.FeeWalletPass == "" {
		return nil, errors.New("the feewalletpass option is not set")
	}

	// Ensure the fee dcrwallet RPC cert path is set.
	if cfg.FeeWalletCert == "" {
		return nil, errors.New("the feewalletcert option is not set")
	}

	// Load fee dcrwallet RPC certificate.
	cfg.FeeWalletCert = cleanAndExpandPath(cfg.FeeWalletCert)
	cfg.feeWalletCert, err = ioutil.ReadFile(cfg.FeeWalletCert)
	if err != nil {
		return nil, fmt.Errorf("failed to read fee dcrwallet cert file: %v", err)
	}

	// Ensure the voting dcrwallet RPC username is set.
	if cfg.VotingWalletUser == "" {
		return nil, errors.New("the votingwalletuser option is not set")
	}

	// Ensure the voting dcrwallet RPC password is set.
	if cfg.VotingWalletPass == "" {
		return nil, errors.New("the votingwalletpass option is not set")
	}

	// Ensure the voting dcrwallet RPC cert path is set.
	if cfg.VotingWalletCert == "" {
		return nil, errors.New("the votingwalletcert option is not set")
	}

	// Load voting dcrwallet RPC certificate.
	cfg.VotingWalletCert = cleanAndExpandPath(cfg.VotingWalletCert)
	cfg.votingWalletCert, err = ioutil.ReadFile(cfg.VotingWalletCert)
	if err != nil {
		return nil, fmt.Errorf("failed to read voting dcrwallet cert file: %v", err)
	}

	// Add default port for the active network if there is no port specified.
	cfg.FeeWalletHost = normalizeAddress(cfg.FeeWalletHost, cfg.netParams.WalletRPCServerPort)
	cfg.VotingWalletHost = normalizeAddress(cfg.VotingWalletHost, cfg.netParams.WalletRPCServerPort)

	// Create the data directory.
	dataDir := filepath.Join(cfg.HomeDir, "data", cfg.netParams.Name)
	err = os.MkdirAll(dataDir, 0700)
	if err != nil {
		return nil, fmt.Errorf("failed to create data directory: %v", err)
	}

	// Initialize loggers and log rotation.
	logDir := filepath.Join(cfg.HomeDir, "logs", cfg.netParams.Name)
	initLogRotator(filepath.Join(logDir, "dcrvsp.log"))
	setLogLevels(cfg.LogLevel)

	// Set the database path
	cfg.dbPath = filepath.Join(dataDir, "vsp.db")

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
