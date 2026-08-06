// Package ipfs is a wrapper around IPFS UnixFS API.
package ipfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	files "github.com/ipfs/boxo/files"
	icorepath "github.com/ipfs/boxo/path"
	"github.com/ipfs/go-cid"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/coreapi"
	icore "github.com/ipfs/kubo/core/coreiface"
	"github.com/ipfs/kubo/core/coreiface/options"
	libp2p "github.com/ipfs/kubo/core/node/libp2p"
	"github.com/ipfs/kubo/plugin/loader" // This package is needed so that all the preloaded plugins are loaded automatically
	"github.com/ipfs/kubo/repo/fsrepo"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Plugin registration is process-global state in kubo's loader package, so
// it must only happen once even if multiple IPFS nodes are spawned in the
// same process (e.g. across tests).
var (
	pluginsOnce sync.Once
	pluginsErr  error
)

func setupPlugins(externalPluginsPath string) error {
	pluginsOnce.Do(func() {
		// Load any external plugins if available on externalPluginsPath
		plugins, err := loader.NewPluginLoader(filepath.Join(externalPluginsPath, "plugins"))
		if err != nil {
			pluginsErr = fmt.Errorf("error loading plugins: %s", err)
			return
		}

		// Load preloaded and external plugins
		if err := plugins.Initialize(); err != nil {
			pluginsErr = fmt.Errorf("error initializing plugins: %s", err)
			return
		}
		if err := plugins.Inject(); err != nil {
			pluginsErr = fmt.Errorf("error initializing plugins: %s", err)
			return
		}
	})
	return pluginsErr
}

func createTempRepo(ctx context.Context, repoPath string) (string, error) {
	// Create a config with default options and a 2048 bit key
	cfg, err := config.Init(ioutil.Discard, 2048)
	if err != nil {
		return "", err
	}

	// Create the repo with the config
	err = fsrepo.Init(repoPath, cfg)
	if err != nil {
		return "", fmt.Errorf("failed to init ephemeral node: %s", err)
	}
	return repoPath, nil
}

// createNode creates an IPFS node and returns its coreAPI.
func createNode(ctx context.Context, repoPath string) (icore.CoreAPI, error) {
	// Open the repo
	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		return nil, err
	}

	// Construct the node
	nodeOptions := &core.BuildCfg{
		Online:  true,
		Routing: libp2p.DHTOption, // This option sets the node to be a full DHT node (both fetching and storing DHT Records)
		// Routing: libp2p.DHTClientOption, // This option sets the node to be a client DHT node (only fetching records)
		Repo: repo,
	}
	node, err := core.NewNode(ctx, nodeOptions)
	if err != nil {
		return nil, err
	}

	// Attach the Core API to the constructed node
	return coreapi.NewCoreAPI(node)
}

// Spawns a node on the default repo location, if the repo exists
func spawnDefault(ctx context.Context) (icore.CoreAPI, error) {
	defaultPath, err := config.PathRoot()
	if err != nil {
		// shouldn't be possible
		return nil, err
	}
	if err := setupPlugins(defaultPath); err != nil {
		return nil, err

	}
	return createNode(ctx, defaultPath)
}

// Spawns a node to be used just for this run (i.e. creates a tmp repo)
func spawnEphemeral(ctx context.Context, repoPath string) (icore.CoreAPI, error) {
	if err := setupPlugins(""); err != nil {
		return nil, err
	}

	// Create a Temporary Repo
	repoPath, err := createTempRepo(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create temp repo: %s", err)
	}

	// Spawning an ephemeral IPFS node
	return createNode(ctx, repoPath)
}

func getPeers() (map[peer.ID]*peer.AddrInfo, error) {
	addrs := []string{
		// IPFS Bootstrapper nodes.
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
		"/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
		"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
		"/ip4/104.131.131.82/udp/4001/quic/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",

		// IPFS Cluster Pinning nodes
		"/ip4/138.201.67.219/tcp/4001/p2p/QmUd6zHcbkbcs7SMxwLs48qZVX3vpcM8errYS7xEczwRMA",
		"/ip4/138.201.67.219/udp/4001/quic/p2p/QmUd6zHcbkbcs7SMxwLs48qZVX3vpcM8errYS7xEczwRMA",
		"/ip4/138.201.67.220/tcp/4001/p2p/QmNSYxZAiJHeLdkBg38roksAR9So7Y5eojks1yjEcUtZ7i",
		"/ip4/138.201.67.220/udp/4001/quic/p2p/QmNSYxZAiJHeLdkBg38roksAR9So7Y5eojks1yjEcUtZ7i",
		"/ip4/138.201.68.74/tcp/4001/p2p/QmdnXwLrC8p1ueiq2Qya8joNvk3TVVDAut7PrikmZwubtR",
		"/ip4/138.201.68.74/udp/4001/quic/p2p/QmdnXwLrC8p1ueiq2Qya8joNvk3TVVDAut7PrikmZwubtR",
		"/ip4/94.130.135.167/tcp/4001/p2p/QmUEMvxS2e7iDrereVYc5SWPauXPyNwxcy9BXZrC1QTcHE",
		"/ip4/94.130.135.167/udp/4001/quic/p2p/QmUEMvxS2e7iDrereVYc5SWPauXPyNwxcy9BXZrC1QTcHE",
	}

	peers := make(map[peer.ID]*peer.AddrInfo, len(addrs))
	for _, addrStr := range addrs {
		addr, err := ma.NewMultiaddr(addrStr)
		if err != nil {
			return nil, err
		}
		pii, err := peer.AddrInfoFromP2pAddr(addr)
		if err != nil {
			return nil, err
		}
		pi, ok := peers[pii.ID]
		if !ok {
			pi = &peer.AddrInfo{ID: pii.ID}
			peers[pi.ID] = pi
		}
		pi.Addrs = append(pi.Addrs, pii.Addrs...)
	}
	return peers, nil
}

func connectToPeers(ctx context.Context, ipfs icore.CoreAPI, peers map[peer.ID]*peer.AddrInfo, logger *log.Logger) {
	var wg sync.WaitGroup

	wg.Add(len(peers))
	for _, info := range peers {
		go func(info *peer.AddrInfo) {
			defer wg.Done()
			err := ipfs.Swarm().Connect(ctx, *info)
			if err != nil {
				logger.Printf("failed to connect to %s: %s", info.ID, err)
			}
		}(info)
	}
	wg.Wait()
}

func getUnixfsNode(path string) (files.Node, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	return files.NewSerialFile(path, false, st)
}

type IPFS struct {
	core icore.CoreAPI
	root *DirEntry
	cfg  *Config
}

type Config struct {
	LogWriter   io.Writer
	TempRepo    bool
	TempRepoDir string // IPFS repository directory if TempRepo == true
}

func defaultConfig(cfg *Config) *Config {
	if cfg == nil {
		cfg = &Config{
			LogWriter:   io.Discard,
			TempRepo:    false,
			TempRepoDir: "",
		}
	}
	if cfg.LogWriter == nil {
		cfg.LogWriter = io.Discard
	}
	return cfg
}

func New(ctx context.Context, cfg *Config) (*IPFS, error) {
	cfg = defaultConfig(cfg)

	var core icore.CoreAPI
	if cfg.TempRepo {
		// Spawn a node using a temporary path, creating a temporary repo for the run
		var err error
		core, err = spawnEphemeral(ctx, cfg.TempRepoDir)
		if err != nil {
			return nil, fmt.Errorf("failed to spawn ephemeral node: %v", err)
		}
	} else {
		// Spawn a node using the default path (~/.ipfs), assuming that a repo exists there already
		var err error
		core, err = spawnDefault(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not spawn IPFS using the default path: %v", err)
		}
	}

	peers, err := getPeers()
	if err != nil {
		return nil, err
	}
	logger := log.New(cfg.LogWriter, "", log.LstdFlags)
	go connectToPeers(ctx, core, peers, logger)

	root := &DirEntry{
		Name:   "",
		Cid:    cid.Cid{},
		Size:   0,
		Type:   icore.TDirectory,
		Parent: nil,
	}
	root.Parent = root
	return &IPFS{
		core: core,
		root: root,
		cfg:  cfg,
	}, nil
}

func nodeType(node files.Node) icore.FileType {
	switch node.(type) {
	default:
		return icore.TUnknown
	case *files.Symlink:
		return icore.TSymlink
	case files.File:
		return icore.TFile
	case files.Directory:
		return icore.TDirectory
	}
}

type DirEntry struct {
	Name   string
	Cid    cid.Cid
	Size   uint64
	Type   icore.FileType
	Parent *DirEntry
}

func (ipfs *IPFS) Root() *DirEntry {
	return ipfs.root
}

func (ipfs *IPFS) walkRoot(ctx context.Context, name string) (*DirEntry, error) {
	// name is a single path component (see Walk), typically a bare CID; the
	// path parser requires a namespace prefix.
	p, err := icorepath.NewPath("/ipfs/" + name)
	if err != nil {
		return nil, err
	}
	resolved, _, err := ipfs.core.ResolvePath(ctx, p)
	if err != nil {
		return nil, err
	}
	name = resolved.RootCid().String() // normalize
	node, err := ipfs.core.Unixfs().Get(ctx, resolved)
	if err != nil {
		return nil, err
	}
	defer node.Close()

	f := &DirEntry{
		Name:   name,
		Cid:    resolved.RootCid(),
		Size:   0,
		Type:   nodeType(node),
		Parent: nil,
	}
	f.Parent = ipfs.root
	return f, nil
}

func (ipfs *IPFS) Walk(ctx context.Context, f *DirEntry, name string) (*DirEntry, error) {
	if name == ".." {
		return f.Parent, nil
	}
	if strings.ContainsRune(name, '/') {
		return nil, fmt.Errorf("walk target contains slash")
	}
	if f.Type != icore.TDirectory {
		return nil, errors.New("not a directory")
	}
	if f == ipfs.root {
		return ipfs.walkRoot(ctx, name)
	}
	p, err := icorepath.Join(icorepath.FromCid(f.Cid), name)
	if err != nil {
		return nil, err
	}
	resolved, remainder, err := ipfs.core.ResolvePath(ctx, p)
	if err != nil {
		return nil, err
	}
	if len(remainder) != 0 {
		return nil, fmt.Errorf("could not fully resolve path %v", p)
	}
	node, err := ipfs.core.Unixfs().Get(ctx, resolved)
	if err != nil {
		return nil, err
	}
	defer node.Close()

	return &DirEntry{
		Name:   name,
		Cid:    resolved.RootCid(),
		Size:   0,
		Type:   nodeType(node),
		Parent: f,
	}, nil
}

func (ipfs *IPFS) GetFile(ctx context.Context, f *DirEntry) (files.File, error) {
	if f.Type != icore.TFile {
		return nil, errors.New("not a regular file")
	}
	node, err := ipfs.core.Unixfs().Get(ctx, icorepath.FromCid(f.Cid))
	if err != nil {
		return nil, err
	}
	return node.(files.File), nil
}

func (ipfs *IPFS) ReadDir(ctx context.Context, f *DirEntry) ([]DirEntry, error) {
	if f.Type != icore.TDirectory {
		return nil, errors.New("not a directory")
	}
	if f == ipfs.root {
		return nil, nil
	}
	entries := make([]DirEntry, 0)
	for entry, err := range icore.LsIter(ctx, ipfs.core.Unixfs(), icorepath.FromCid(f.Cid), options.Unixfs.ResolveChildren(true)) {
		if err != nil {
			return nil, err
		}
		entries = append(entries, DirEntry{
			Name:   entry.Name,
			Cid:    entry.Cid,
			Size:   entry.Size,
			Type:   entry.Type,
			Parent: f,
		})
	}
	return entries, nil
}

func (ipfs *IPFS) Put(ctx context.Context, filename string) (*DirEntry, error) {
	node, err := getUnixfsNode(filename)
	if err != nil {
		return nil, err
	}
	defer node.Close()

	resolved, err := ipfs.core.Unixfs().Add(ctx, node)
	if err != nil {
		return nil, err
	}
	size, err := node.Size()
	if err != nil {
		return nil, err
	}
	e := &DirEntry{
		Name:   filepath.Base(filename),
		Cid:    resolved.RootCid(),
		Size:   uint64(size),
		Type:   nodeType(node),
		Parent: nil,
	}
	e.Parent = e
	return e, nil
}
