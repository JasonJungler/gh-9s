package ghcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Everything here changes a wiki: a save, a new page and a delete are each one commit pushed straight to the
// wiki's default branch (a wiki has no pull requests). Each starts by bringing the local copy up to date, so the
// commit goes on top of what GitHub has now; and each keeps the user's text when it cannot finish.

// wikiRecover keeps text the user wrote that could not be saved, and returns where it is.
func wikiRecover(tg RepoRef, file, text string) string {
	dir := filepath.Join(wikiRoot(), "lost", strings.ReplaceAll(hostOf(tg), ":", "_"), tg.Owner, tg.Name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	p := filepath.Join(dir, file+"."+time.Now().Format("20060102-150405"))
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		return ""
	}
	return p
}

// lostNote words where the text went (or, if even that failed, says so).
func lostNote(path string) string {
	if path == "" {
		return "your text could not be kept either; copy it from the editor next time"
	}
	return "your text is saved in " + path
}

// wikiIdentity returns the -c settings for the commit: the user's own git name and email, and for what is
// missing the login's GitHub no-reply address.
func wikiIdentity(dir string, tg RepoRef) ([]string, error) {
	name, _ := wikiGit(dir, "config", "user.name")
	email, _ := wikiGit(dir, "config", "user.email")
	n, e := strings.TrimSpace(string(name)), strings.TrimSpace(string(email))
	if n != "" && e != "" {
		return nil, nil
	}
	login, err := wikiLogin(hostOf(tg))
	login = strings.TrimSpace(login)
	if err != nil || login == "" || !namePart.MatchString(login) {
		return nil, errors.New("git has no user.name / user.email and GitHub's login could not be read; set them with git config --global")
	}
	domain := "users.noreply.github.com"
	if h := hostOf(tg); h != "github.com" {
		domain = "users.noreply." + h
	}
	var args []string
	if n == "" {
		args = append(args, "-c", "user.name="+login)
	}
	if e == "" {
		args = append(args, "-c", "user.email="+login+"@"+domain)
	}
	return args, nil
}

// wikiPublish commits what is staged in the copy and pushes it. A failure leaves the copy with work that never
// reached GitHub; the next call starts by bringing the copy back in step, so nothing stays half done.
func wikiPublish(dir, branch string, tg RepoRef, message string) error {
	id, err := wikiIdentity(dir, tg)
	if err != nil {
		return err
	}
	if _, err := wikiGit(dir, append(id, "commit", "--quiet", "-m", message)...); err != nil {
		return fmt.Errorf("could not record the change: %s", firstLine(CleanText(err.Error())))
	}
	if _, err := wikiGit(dir, "push", "--quiet", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return fmt.Errorf("GitHub did not accept the change (no write access, or someone saved first): %s", firstLine(CleanText(err.Error())))
	}
	return nil
}

// wikiPrepare is the start every change shares: check the inputs, bring the copy up to date, read its files.
func wikiPrepare(tg RepoRef, message string) (dir, branch string, es []wikiEntry, err error) {
	if strings.TrimSpace(message) == "" {
		return "", "", nil, errors.New("a change needs a message")
	}
	var st WikiState
	if dir, branch, st, err = wikiSync(tg); err != nil {
		return
	}
	if st == WikiOff {
		return "", "", nil, errors.New("the wiki is turned off for this repo")
	}
	if st == WikiNoPages {
		return "", "", nil, errors.New("this wiki has no pages yet: GitHub creates it with its first page, which has to be made in the browser (o)")
	}
	es, err = wikiTree(dir)
	return
}

func findEntry(es []wikiEntry, file string) (wikiEntry, bool) {
	for _, e := range es {
		if e.file == file {
			return e, true
		}
	}
	return wikiEntry{}, false
}

func checkWikiText(text string) error {
	if len(text) > WikiMaxBytes {
		return errors.New("a page can be at most 1 MB")
	}
	return nil
}

// SaveWikiPage replaces a page's text. baseBlob is the version the user started from: if the page on GitHub is
// no longer that, nothing is saved and the user's text is kept in a file.
func SaveWikiPage(tg RepoRef, file, baseBlob, text, message string) error {
	if _, _, ok := wikiFileExt(file); !ok {
		return fmt.Errorf("%q is not a page", file)
	}
	if err := checkWikiText(text); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("a page cannot be emptied here; use x to delete it")
	}
	wikiMu.Lock()
	defer wikiMu.Unlock()
	dir, branch, es, err := wikiPrepare(tg, message)
	if err != nil {
		return err
	}
	e, ok := findEntry(es, file)
	switch {
	case !ok:
		return fmt.Errorf("%s is no longer in the wiki; %s", WikiTitle(file), lostNote(wikiRecover(tg, file, text)))
	case e.link:
		return errors.New("this page is a symbolic link; gh-9s does not write through those")
	case e.blob != baseBlob:
		return fmt.Errorf("%s changed on GitHub since you opened it, so nothing was saved; %s", WikiTitle(file), lostNote(wikiRecover(tg, file, text)))
	}
	if cur, err := wikiGit(dir, "cat-file", "blob", e.blob); err == nil && string(cur) == text {
		return errors.New("no changes to save")
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(text), 0o644); err != nil {
		return err
	}
	if _, err := wikiGit(dir, "add", "--", file); err != nil {
		return fmt.Errorf("%s; %s", firstLine(CleanText(err.Error())), lostNote(wikiRecover(tg, file, text)))
	}
	if err := wikiPublish(dir, branch, tg, message); err != nil {
		return fmt.Errorf("%s; %s", err, lostNote(wikiRecover(tg, file, text)))
	}
	return nil
}

// CreateWikiPage adds a page that does not exist yet (no page with the same title, whatever the case or format).
func CreateWikiPage(tg RepoRef, file, text, message string) error {
	if _, _, ok := wikiFileExt(file); !ok || strings.ContainsAny(file, "/\\") {
		return fmt.Errorf("%q is not a page file name", file)
	}
	if err := checkWikiText(text); err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("nothing was written, so no page was created")
	}
	wikiMu.Lock()
	defer wikiMu.Unlock()
	dir, branch, es, err := wikiPrepare(tg, message)
	if err != nil {
		return err
	}
	stem := strings.ToLower(strings.TrimSuffix(file, filepath.Ext(file)))
	for _, e := range es {
		if strings.ToLower(strings.TrimSuffix(e.file, filepath.Ext(e.file))) == stem {
			return fmt.Errorf("a page called %q already exists; %s", WikiTitle(e.file), lostNote(wikiRecover(tg, file, text)))
		}
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(text), 0o644); err != nil {
		return err
	}
	if _, err := wikiGit(dir, "add", "--", file); err != nil {
		return fmt.Errorf("%s; %s", firstLine(CleanText(err.Error())), lostNote(wikiRecover(tg, file, text)))
	}
	if err := wikiPublish(dir, branch, tg, message); err != nil {
		return fmt.Errorf("%s; %s", err, lostNote(wikiRecover(tg, file, text)))
	}
	return nil
}

// DeleteWikiPage removes a page. baseBlob is the version the user was looking at: a page that changed since is
// not deleted. The page stays in the wiki's git history.
func DeleteWikiPage(tg RepoRef, file, baseBlob, message string) error {
	if _, _, ok := wikiFileExt(file); !ok {
		return fmt.Errorf("%q is not a page", file)
	}
	wikiMu.Lock()
	defer wikiMu.Unlock()
	dir, branch, es, err := wikiPrepare(tg, message)
	if err != nil {
		return err
	}
	e, ok := findEntry(es, file)
	switch {
	case !ok:
		return fmt.Errorf("%s is already gone from the wiki", WikiTitle(file))
	case e.blob != baseBlob:
		return fmt.Errorf("%s changed on GitHub since you looked at it, so it was not deleted; press R and look again", WikiTitle(file))
	}
	if _, err := wikiGit(dir, "rm", "--quiet", "--", file); err != nil {
		return errors.New(firstLine(CleanText(err.Error())))
	}
	return wikiPublish(dir, branch, tg, message)
}
