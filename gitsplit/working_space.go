package gitsplit

import (
	"github.com/jderusse/gitsplit/utils"
	"github.com/libgit2/git2go"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"os"
)

type WorkingSpaceFactory struct {
}

type WorkingSpace struct {
	config     Config
	repository *git.Repository
	remotes    *GitRemoteCollection
}

func NewWorkingSpaceFactory() *WorkingSpaceFactory {
	return &WorkingSpaceFactory{}
}

func (w *WorkingSpaceFactory) CreateWorkingSpace(config Config) (*WorkingSpace, error) {
	repository, err := w.getRepository(config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create working repository")
	}

	workingSpace := &WorkingSpace{
		config:     config,
		repository: repository,
		remotes:    NewGitRemoteCollection(repository),
	}

	if err := workingSpace.Init(); err != nil {
		return nil, err
	}

	return workingSpace, nil
}

func (w *WorkingSpaceFactory) getRepository(config Config) (*git.Repository, error) {
	repoPath, err := ioutil.TempDir("", "gitsplit_")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create working directory")
	}
	if config.CacheUrl != nil && config.CacheUrl.IsLocal() {
		repository, err := git.InitRepository(config.CacheUrl.SchemelessUrl(), true)
		if err != nil {
			return nil, errors.Wrap(err, "failed to initialize cache repository")
		}
		repository.Free()

		if err := utils.Copy(config.CacheUrl.SchemelessUrl(), repoPath); err != nil {
			return nil, errors.Wrap(err, "failed to create working space from cache")
		}

		return git.OpenRepository(repoPath)
	}

	log.WithFields(log.Fields{
	    "path": repoPath,
	}).Info("Create new repository")
	return git.InitRepository(repoPath, true)
}

func (w *WorkingSpace) GetCachePool() (CachePoolInterface, error) {
	if w.config.CacheUrl == nil {
		return &NullCachePool{}, nil
	}

	remote, err := w.Remotes().Get("cache")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create cache pool")
	}

	return NewCachePool(w.repository.Path(), remote), nil
}

func (w *WorkingSpace) Repository() *git.Repository {
	return w.repository
}

func (w *WorkingSpace) Remotes() *GitRemoteCollection {
	return w.remotes
}

func (w *WorkingSpace) Init() error {
	if w.config.CacheUrl != nil && !utils.FileExists(w.config.CacheUrl.SchemelessUrl()) {
		log.WithFields(log.Fields{
		    "path": w.config.CacheUrl.SchemelessUrl(),
		}).Info("Initializing repository")
		repository, err := git.InitRepository(w.config.CacheUrl.SchemelessUrl(), true)
		if err != nil {
			return errors.Wrap(err, "failed to initialize working space")
		}
		repository.Free()
	}
	if w.config.CacheUrl != nil {
		w.remotes.Add("cache", w.config.CacheUrl.Url(), []string{"split"}).Fetch()
	}
	w.remotes.Add("origin", w.config.ProjectUrl.Url(), []string{"heads", "tags"}).Fetch()

	for _, split := range w.config.Splits {
		for _, target := range split.Targets {
			w.remotes.Add(target, target, []string{"heads", "tags"})
		}
	}
	go w.remotes.Clean()

	if err := w.remotes.Flush(); err != nil {
		return err
	}

	return nil
}

func (w *WorkingSpace) Close() {
	if err := w.remotes.Flush(); err != nil {
		log.Fatal(err)
	}
	os.RemoveAll(w.repository.Path())
	w.repository.Free()
}
