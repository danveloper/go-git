package git

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/src-d/go-git-fixtures"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/storer"
	"gopkg.in/src-d/go-git.v4/storage"
	"gopkg.in/src-d/go-git.v4/storage/filesystem"
	"gopkg.in/src-d/go-git.v4/storage/memory"

	. "gopkg.in/check.v1"
	"gopkg.in/src-d/go-billy.v3/osfs"
)

type RemoteSuite struct {
	BaseSuite
}

var _ = Suite(&RemoteSuite{})

func (s *RemoteSuite) TestFetchInvalidEndpoint(c *C) {
	r := newRemote(nil, &config.RemoteConfig{Name: "foo", URL: "http://\\"})
	err := r.Fetch(&FetchOptions{RemoteName: "foo"})
	c.Assert(err, ErrorMatches, ".*invalid character.*")
}

func (s *RemoteSuite) TestFetchNonExistentEndpoint(c *C) {
	r := newRemote(nil, &config.RemoteConfig{Name: "foo", URL: "ssh://non-existent/foo.git"})
	err := r.Fetch(&FetchOptions{})
	c.Assert(err, NotNil)
}

func (s *RemoteSuite) TestFetchInvalidSchemaEndpoint(c *C) {
	r := newRemote(nil, &config.RemoteConfig{Name: "foo", URL: "qux://foo"})
	err := r.Fetch(&FetchOptions{})
	c.Assert(err, ErrorMatches, ".*unsupported scheme.*")
}

func (s *RemoteSuite) TestFetchInvalidFetchOptions(c *C) {
	r := newRemote(nil, &config.RemoteConfig{Name: "foo", URL: "qux://foo"})
	invalid := config.RefSpec("^*$ñ")
	err := r.Fetch(&FetchOptions{RefSpecs: []config.RefSpec{invalid}})
	c.Assert(err, Equals, config.ErrRefSpecMalformedSeparator)
}

func (s *RemoteSuite) TestFetchWildcard(c *C) {
	r := newRemote(memory.NewStorage(), &config.RemoteConfig{
		URL: s.GetBasicLocalRepositoryURL(),
	})

	s.testFetch(c, r, &FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
		},
	}, []*plumbing.Reference{
		plumbing.NewReferenceFromStrings("refs/remotes/origin/master", "6ecf0ef2c2dffb796033e5a02219af86ec6584e5"),
		plumbing.NewReferenceFromStrings("refs/remotes/origin/branch", "e8d3ffab552895c19b9fcf7aa264d277cde33881"),
		plumbing.NewReferenceFromStrings("refs/tags/v1.0.0", "6ecf0ef2c2dffb796033e5a02219af86ec6584e5"),
	})
}

func (s *RemoteSuite) TestFetchWildcardTags(c *C) {
	r := newRemote(memory.NewStorage(), &config.RemoteConfig{
		URL: s.GetLocalRepositoryURL(fixtures.ByTag("tags").One()),
	})

	s.testFetch(c, r, &FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
		},
	}, []*plumbing.Reference{
		plumbing.NewReferenceFromStrings("refs/remotes/origin/master", "f7b877701fbf855b44c0a9e86f3fdce2c298b07f"),
		plumbing.NewReferenceFromStrings("refs/tags/annotated-tag", "b742a2a9fa0afcfa9a6fad080980fbc26b007c69"),
		plumbing.NewReferenceFromStrings("refs/tags/tree-tag", "152175bf7e5580299fa1f0ba41ef6474cc043b70"),
		plumbing.NewReferenceFromStrings("refs/tags/commit-tag", "ad7897c0fb8e7d9a9ba41fa66072cf06095a6cfc"),
		plumbing.NewReferenceFromStrings("refs/tags/blob-tag", "fe6cb94756faa81e5ed9240f9191b833db5f40ae"),
		plumbing.NewReferenceFromStrings("refs/tags/lightweight-tag", "f7b877701fbf855b44c0a9e86f3fdce2c298b07f"),
	})
}

func (s *RemoteSuite) TestFetch(c *C) {
	r := newRemote(memory.NewStorage(), &config.RemoteConfig{
		URL: s.GetLocalRepositoryURL(fixtures.ByTag("tags").One()),
	})

	s.testFetch(c, r, &FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/master:refs/remotes/origin/master"),
		},
	}, []*plumbing.Reference{
		plumbing.NewReferenceFromStrings("refs/remotes/origin/master", "f7b877701fbf855b44c0a9e86f3fdce2c298b07f"),
	})
}

func (s *RemoteSuite) TestFetchWithAllTags(c *C) {
	r := newRemote(memory.NewStorage(), &config.RemoteConfig{
		URL: s.GetLocalRepositoryURL(fixtures.ByTag("tags").One()),
	})

	s.testFetch(c, r, &FetchOptions{
		Tags: AllTags,
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/master:refs/remotes/origin/master"),
		},
	}, []*plumbing.Reference{
		plumbing.NewReferenceFromStrings("refs/remotes/origin/master", "f7b877701fbf855b44c0a9e86f3fdce2c298b07f"),
		plumbing.NewReferenceFromStrings("refs/tags/annotated-tag", "b742a2a9fa0afcfa9a6fad080980fbc26b007c69"),
		plumbing.NewReferenceFromStrings("refs/tags/tree-tag", "152175bf7e5580299fa1f0ba41ef6474cc043b70"),
		plumbing.NewReferenceFromStrings("refs/tags/commit-tag", "ad7897c0fb8e7d9a9ba41fa66072cf06095a6cfc"),
		plumbing.NewReferenceFromStrings("refs/tags/blob-tag", "fe6cb94756faa81e5ed9240f9191b833db5f40ae"),
		plumbing.NewReferenceFromStrings("refs/tags/lightweight-tag", "f7b877701fbf855b44c0a9e86f3fdce2c298b07f"),
	})
}

func (s *RemoteSuite) TestFetchWithDepth(c *C) {
	r := newRemote(memory.NewStorage(), &config.RemoteConfig{
		URL: s.GetBasicLocalRepositoryURL(),
	})

	s.testFetch(c, r, &FetchOptions{
		Depth: 1,
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
		},
	}, []*plumbing.Reference{
		plumbing.NewReferenceFromStrings("refs/remotes/origin/master", "6ecf0ef2c2dffb796033e5a02219af86ec6584e5"),
		plumbing.NewReferenceFromStrings("refs/remotes/origin/branch", "e8d3ffab552895c19b9fcf7aa264d277cde33881"),
		plumbing.NewReferenceFromStrings("refs/tags/v1.0.0", "6ecf0ef2c2dffb796033e5a02219af86ec6584e5"),
	})

	c.Assert(r.s.(*memory.Storage).Objects, HasLen, 18)
}

func (s *RemoteSuite) testFetch(c *C, r *Remote, o *FetchOptions, expected []*plumbing.Reference) {
	err := r.Fetch(o)
	c.Assert(err, IsNil)

	var refs int
	l, err := r.s.IterReferences()
	l.ForEach(func(r *plumbing.Reference) error { refs++; return nil })

	c.Assert(refs, Equals, len(expected))

	for _, exp := range expected {
		r, err := r.s.Reference(exp.Name())
		c.Assert(err, IsNil)
		c.Assert(exp.String(), Equals, r.String())
	}
}

func (s *RemoteSuite) TestFetchWithProgress(c *C) {
	url := s.GetBasicLocalRepositoryURL()
	sto := memory.NewStorage()
	buf := bytes.NewBuffer(nil)

	r := newRemote(sto, &config.RemoteConfig{Name: "foo", URL: url})

	refspec := config.RefSpec("+refs/heads/*:refs/remotes/origin/*")
	err := r.Fetch(&FetchOptions{
		RefSpecs: []config.RefSpec{refspec},
		Progress: buf,
	})

	c.Assert(err, IsNil)
	c.Assert(sto.Objects, HasLen, 31)

	c.Assert(buf.Len(), Not(Equals), 0)
}

type mockPackfileWriter struct {
	storage.Storer
	PackfileWriterCalled bool
}

func (m *mockPackfileWriter) PackfileWriter() (io.WriteCloser, error) {
	m.PackfileWriterCalled = true
	return m.Storer.(storer.PackfileWriter).PackfileWriter()
}

func (s *RemoteSuite) TestFetchWithPackfileWriter(c *C) {
	dir, err := ioutil.TempDir("", "fetch")
	c.Assert(err, IsNil)

	defer os.RemoveAll(dir) // clean up

	fss, err := filesystem.NewStorage(osfs.New(dir))
	c.Assert(err, IsNil)

	mock := &mockPackfileWriter{Storer: fss}

	url := s.GetBasicLocalRepositoryURL()
	r := newRemote(mock, &config.RemoteConfig{Name: "foo", URL: url})

	refspec := config.RefSpec("+refs/heads/*:refs/remotes/origin/*")
	err = r.Fetch(&FetchOptions{
		RefSpecs: []config.RefSpec{refspec},
	})

	c.Assert(err, IsNil)

	var count int
	iter, err := mock.IterEncodedObjects(plumbing.AnyObject)
	c.Assert(err, IsNil)

	iter.ForEach(func(plumbing.EncodedObject) error {
		count++
		return nil
	})

	c.Assert(count, Equals, 31)
	c.Assert(mock.PackfileWriterCalled, Equals, true)
}

func (s *RemoteSuite) TestFetchNoErrAlreadyUpToDate(c *C) {
	url := s.GetBasicLocalRepositoryURL()
	s.doTestFetchNoErrAlreadyUpToDate(c, url)
}

func (s *RemoteSuite) TestFetchNoErrAlreadyUpToDateButStillUpdateLocalRemoteRefs(c *C) {
	r := newRemote(memory.NewStorage(), &config.RemoteConfig{
		URL: s.GetBasicLocalRepositoryURL(),
	})

	o := &FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
		},
	}

	err := r.Fetch(o)
	c.Assert(err, IsNil)

	// Simulate an out of date remote ref even though we have the new commit locally
	r.s.SetReference(plumbing.NewReferenceFromStrings(
		"refs/remotes/origin/master", "918c48b83bd081e863dbe1b80f8998f058cd8294",
	))

	err = r.Fetch(o)
	c.Assert(err, IsNil)

	exp := plumbing.NewReferenceFromStrings(
		"refs/remotes/origin/master", "6ecf0ef2c2dffb796033e5a02219af86ec6584e5",
	)

	ref, err := r.s.Reference("refs/remotes/origin/master")
	c.Assert(err, IsNil)
	c.Assert(exp.String(), Equals, ref.String())
}

func (s *RemoteSuite) TestFetchNoErrAlreadyUpToDateWithNonCommitObjects(c *C) {
	fixture := fixtures.ByTag("tags").One()
	url := s.GetLocalRepositoryURL(fixture)
	s.doTestFetchNoErrAlreadyUpToDate(c, url)
}

func (s *RemoteSuite) doTestFetchNoErrAlreadyUpToDate(c *C, url string) {
	r := newRemote(memory.NewStorage(), &config.RemoteConfig{URL: url})

	o := &FetchOptions{
		RefSpecs: []config.RefSpec{
			config.RefSpec("+refs/heads/*:refs/remotes/origin/*"),
		},
	}

	err := r.Fetch(o)
	c.Assert(err, IsNil)
	err = r.Fetch(o)
	c.Assert(err, Equals, NoErrAlreadyUpToDate)
}

func (s *RemoteSuite) TestString(c *C) {
	r := newRemote(nil, &config.RemoteConfig{
		Name: "foo",
		URL:  "https://github.com/git-fixtures/basic.git",
	})

	c.Assert(r.String(), Equals, ""+
		"foo\thttps://github.com/git-fixtures/basic.git (fetch)\n"+
		"foo\thttps://github.com/git-fixtures/basic.git (push)",
	)
}

func (s *RemoteSuite) TestPushToEmptyRepository(c *C) {
	srcFs := fixtures.Basic().One().DotGit()
	sto, err := filesystem.NewStorage(srcFs)
	c.Assert(err, IsNil)

	dstFs := fixtures.ByTag("empty").One().DotGit()
	url := dstFs.Root()

	r := newRemote(sto, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  url,
	})

	rs := config.RefSpec("refs/heads/*:refs/heads/*")
	err = r.Push(&PushOptions{
		RefSpecs: []config.RefSpec{rs},
	})
	c.Assert(err, IsNil)

	dstSto, err := filesystem.NewStorage(dstFs)
	c.Assert(err, IsNil)
	dstRepo, err := Open(dstSto, nil)
	c.Assert(err, IsNil)

	iter, err := sto.IterReferences()
	c.Assert(err, IsNil)
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		if !ref.IsBranch() {
			return nil
		}

		dstRef, err := dstRepo.Reference(ref.Name(), true)
		c.Assert(err, IsNil, Commentf("ref: %s", ref.String()))
		c.Assert(dstRef, DeepEquals, ref)

		return nil
	})
	c.Assert(err, IsNil)
}

func (s *RemoteSuite) TestPushTags(c *C) {
	srcFs := fixtures.ByURL("https://github.com/git-fixtures/tags.git").One().DotGit()
	sto, err := filesystem.NewStorage(srcFs)
	c.Assert(err, IsNil)

	dstFs := fixtures.ByTag("empty").One().DotGit()
	url := dstFs.Root()

	r := newRemote(sto, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  url,
	})

	rs := config.RefSpec("refs/tags/*:refs/tags/*")
	err = r.Push(&PushOptions{
		RefSpecs: []config.RefSpec{rs},
	})
	c.Assert(err, IsNil)

	dstSto, err := filesystem.NewStorage(dstFs)
	c.Assert(err, IsNil)
	dstRepo, err := Open(dstSto, nil)
	c.Assert(err, IsNil)

	ref, err := dstRepo.Storer.Reference(plumbing.ReferenceName("refs/tags/lightweight-tag"))
	c.Assert(err, IsNil)
	c.Assert(ref, DeepEquals, plumbing.NewReferenceFromStrings("refs/tags/lightweight-tag", "f7b877701fbf855b44c0a9e86f3fdce2c298b07f"))

	ref, err = dstRepo.Storer.Reference(plumbing.ReferenceName("refs/tags/annotated-tag"))
	c.Assert(err, IsNil)
	c.Assert(ref, DeepEquals, plumbing.NewReferenceFromStrings("refs/tags/annotated-tag", "b742a2a9fa0afcfa9a6fad080980fbc26b007c69"))

	ref, err = dstRepo.Storer.Reference(plumbing.ReferenceName("refs/tags/commit-tag"))
	c.Assert(err, IsNil)
	c.Assert(ref, DeepEquals, plumbing.NewReferenceFromStrings("refs/tags/commit-tag", "ad7897c0fb8e7d9a9ba41fa66072cf06095a6cfc"))

	ref, err = dstRepo.Storer.Reference(plumbing.ReferenceName("refs/tags/blob-tag"))
	c.Assert(err, IsNil)
	c.Assert(ref, DeepEquals, plumbing.NewReferenceFromStrings("refs/tags/blob-tag", "fe6cb94756faa81e5ed9240f9191b833db5f40ae"))

	ref, err = dstRepo.Storer.Reference(plumbing.ReferenceName("refs/tags/tree-tag"))
	c.Assert(err, IsNil)
	c.Assert(ref, DeepEquals, plumbing.NewReferenceFromStrings("refs/tags/tree-tag", "152175bf7e5580299fa1f0ba41ef6474cc043b70"))
}

func (s *RemoteSuite) TestPushNoErrAlreadyUpToDate(c *C) {
	f := fixtures.Basic().One()
	sto, err := filesystem.NewStorage(f.DotGit())
	c.Assert(err, IsNil)
	url := f.DotGit().Root()
	r := newRemote(sto, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  url,
	})

	rs := config.RefSpec("refs/heads/*:refs/heads/*")
	err = r.Push(&PushOptions{
		RefSpecs: []config.RefSpec{rs},
	})
	c.Assert(err, Equals, NoErrAlreadyUpToDate)
}

func (s *RemoteSuite) TestPushDeleteReference(c *C) {
	f := fixtures.Basic().One()
	sto, err := filesystem.NewStorage(f.DotGit())
	c.Assert(err, IsNil)

	dstFs := f.DotGit()
	dstSto, err := filesystem.NewStorage(dstFs)
	c.Assert(err, IsNil)
	prepareRepo(c, dstFs.Root())

	url := dstFs.Root()
	r := newRemote(sto, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  url,
	})

	rs := config.RefSpec(":refs/heads/branch")
	err = r.Push(&PushOptions{
		RefSpecs: []config.RefSpec{rs},
	})
	c.Assert(err, IsNil)

	_, err = dstSto.Reference(plumbing.ReferenceName("refs/heads/branch"))
	c.Assert(err, Equals, plumbing.ErrReferenceNotFound)
}

func (s *RemoteSuite) TestPushRejectNonFastForward(c *C) {
	f := fixtures.Basic().One()
	sto, err := filesystem.NewStorage(f.DotGit())
	c.Assert(err, IsNil)

	dstFs := f.DotGit()
	dstSto, err := filesystem.NewStorage(dstFs)
	c.Assert(err, IsNil)

	url := dstFs.Root()
	r := newRemote(sto, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  url,
	})

	oldRef, err := dstSto.Reference(plumbing.ReferenceName("refs/heads/branch"))
	c.Assert(err, IsNil)
	c.Assert(oldRef, NotNil)

	err = r.Push(&PushOptions{RefSpecs: []config.RefSpec{
		config.RefSpec("refs/heads/master:refs/heads/branch"),
	}})
	c.Assert(err, ErrorMatches, "non-fast-forward update: refs/heads/branch")

	newRef, err := dstSto.Reference(plumbing.ReferenceName("refs/heads/branch"))
	c.Assert(err, IsNil)
	c.Assert(newRef, DeepEquals, oldRef)
}

func (s *RemoteSuite) TestPushForce(c *C) {
	f := fixtures.Basic().One()
	sto, err := filesystem.NewStorage(f.DotGit())
	c.Assert(err, IsNil)

	dstFs := f.DotGit()
	dstSto, err := filesystem.NewStorage(dstFs)
	c.Assert(err, IsNil)

	url := dstFs.Root()
	r := newRemote(sto, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  url,
	})

	oldRef, err := dstSto.Reference(plumbing.ReferenceName("refs/heads/branch"))
	c.Assert(err, IsNil)
	c.Assert(oldRef, NotNil)

	err = r.Push(&PushOptions{RefSpecs: []config.RefSpec{
		config.RefSpec("+refs/heads/master:refs/heads/branch"),
	}})
	c.Assert(err, IsNil)

	newRef, err := dstSto.Reference(plumbing.ReferenceName("refs/heads/branch"))
	c.Assert(err, IsNil)
	c.Assert(newRef, Not(DeepEquals), oldRef)
}

func (s *RemoteSuite) TestPushNewReference(c *C) {
	f := fixtures.Basic().One()
	sto, err := filesystem.NewStorage(f.DotGit())
	c.Assert(err, IsNil)

	dstFs := f.DotGit()
	dstSto, err := filesystem.NewStorage(dstFs)
	c.Assert(err, IsNil)

	url := dstFs.Root()
	r := newRemote(sto, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  url,
	})

	oldRef, err := dstSto.Reference(plumbing.ReferenceName("refs/heads/branch"))
	c.Assert(err, IsNil)
	c.Assert(oldRef, NotNil)

	err = r.Push(&PushOptions{RefSpecs: []config.RefSpec{
		config.RefSpec("refs/heads/branch:refs/heads/branch2"),
	}})
	c.Assert(err, IsNil)

	newRef, err := dstSto.Reference(plumbing.ReferenceName("refs/heads/branch2"))
	c.Assert(err, IsNil)
	c.Assert(newRef.Hash(), Equals, oldRef.Hash())
}

func (s *RemoteSuite) TestPushInvalidEndpoint(c *C) {
	r := newRemote(nil, &config.RemoteConfig{Name: "foo", URL: "http://\\"})
	err := r.Push(&PushOptions{RemoteName: "foo"})
	c.Assert(err, ErrorMatches, ".*invalid character.*")
}

func (s *RemoteSuite) TestPushNonExistentEndpoint(c *C) {
	r := newRemote(nil, &config.RemoteConfig{Name: "foo", URL: "ssh://non-existent/foo.git"})
	err := r.Push(&PushOptions{})
	c.Assert(err, NotNil)
}

func (s *RemoteSuite) TestPushInvalidSchemaEndpoint(c *C) {
	r := newRemote(nil, &config.RemoteConfig{Name: "foo", URL: "qux://foo"})
	err := r.Push(&PushOptions{})
	c.Assert(err, ErrorMatches, ".*unsupported scheme.*")
}

func (s *RemoteSuite) TestPushInvalidFetchOptions(c *C) {
	r := newRemote(nil, &config.RemoteConfig{Name: "foo", URL: "qux://foo"})
	invalid := config.RefSpec("^*$ñ")
	err := r.Push(&PushOptions{RefSpecs: []config.RefSpec{invalid}})
	c.Assert(err, Equals, config.ErrRefSpecMalformedSeparator)
}

func (s *RemoteSuite) TestPushInvalidRefSpec(c *C) {
	r := newRemote(nil, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  "some-url",
	})

	rs := config.RefSpec("^*$**")
	err := r.Push(&PushOptions{
		RefSpecs: []config.RefSpec{rs},
	})
	c.Assert(err, Equals, config.ErrRefSpecMalformedSeparator)
}

func (s *RemoteSuite) TestPushWrongRemoteName(c *C) {
	r := newRemote(nil, &config.RemoteConfig{
		Name: DefaultRemoteName,
		URL:  "some-url",
	})

	err := r.Push(&PushOptions{
		RemoteName: "other-remote",
	})
	c.Assert(err, ErrorMatches, ".*remote names don't match.*")
}

func (s *RemoteSuite) TestGetHaves(c *C) {
	st := memory.NewStorage()
	st.SetReference(plumbing.NewReferenceFromStrings(
		"foo", "f7b877701fbf855b44c0a9e86f3fdce2c298b07f",
	))

	st.SetReference(plumbing.NewReferenceFromStrings(
		"bar", "fe6cb94756faa81e5ed9240f9191b833db5f40ae",
	))

	st.SetReference(plumbing.NewReferenceFromStrings(
		"qux", "f7b877701fbf855b44c0a9e86f3fdce2c298b07f",
	))

	l, err := getHaves(st)
	c.Assert(err, IsNil)
	c.Assert(l, HasLen, 2)
}

const bareConfig = `[core]
repositoryformatversion = 0
filemode = true
bare = true`

func prepareRepo(c *C, path string) {
	// git-receive-pack refuses to update refs/heads/master on non-bare repo
	// so we ensure bare repo config.
	config := filepath.Join(path, "config")
	if _, err := os.Stat(config); err == nil {
		f, err := os.OpenFile(config, os.O_TRUNC|os.O_WRONLY, 0)
		c.Assert(err, IsNil)
		content := strings.NewReader(bareConfig)
		_, err = io.Copy(f, content)
		c.Assert(err, IsNil)
		c.Assert(f.Close(), IsNil)
	}
}
