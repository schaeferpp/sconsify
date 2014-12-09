package ui

import (
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"
	"strings"

	"github.com/fabiofalci/sconsify/events"
	"github.com/jroimartin/gocui"
	sp "github.com/op/go-libspotify/spotify"
)

var (
	gui       *Gui
	queue     *Queue
	state     *UiState
	playlists map[string]*sp.Playlist
)

type Gui struct {
	g             *gocui.Gui
	playlistsView *gocui.View
	tracksView    *gocui.View
	statusView    *gocui.View
	queueView     *gocui.View
	events        *events.Events
	currentTrack  *sp.Track
}

func StartConsoleUserInterface(events *events.Events) {
	select {
	case playlists = <-events.WaitForPlaylists():
		if playlists == nil {
			return
		}
	case <-events.WaitForShutdown():
		return
	}

	gui = &Gui{events: events}

	queue = InitQueue()
	state = InitState()

	go func() {
		for {
			select {
			case message := <-gui.events.WaitForStatus():
				gui.updateStatus(message)
			case <-gui.events.WaitForPlayTokenLost():
				gui.updateStatus("Play token lost")
			case <-gui.events.NextPlay:
				gui.playNext()
			}
		}
	}()

	gui.g = gocui.NewGui()
	if err := gui.g.Init(); err != nil {
		log.Panicln(err)
	}
	defer gui.g.Close()

	gui.g.SetLayout(layout)
	if err := keybindings(); err != nil {
		log.Panicln(err)
	}
	gui.g.SelBgColor = gocui.ColorGreen
	gui.g.SelFgColor = gocui.ColorBlack
	gui.g.ShowCursor = true

	err := gui.g.MainLoop()
	if err != nil && err != gocui.ErrorQuit {
		log.Panicln(err)
	}
}

func (gui *Gui) updateStatus(message string) {
	gui.statusView.Clear()
	gui.statusView.SetCursor(0, 0)
	gui.statusView.SetOrigin(0, 0)

	state.currentMessage = message
	fmt.Fprintf(gui.statusView, state.getModeAsString()+"%v", state.currentMessage)

	// otherwise the update will appear only in the next keyboard move
	gui.g.Flush()
}

func (gui *Gui) getSelectedPlaylist() (string, error) {
	return gui.getSelected(gui.playlistsView)
}

func (gui *Gui) getSelectedTrack() (string, error) {
	return gui.getSelected(gui.tracksView)
}

func (gui *Gui) getSelected(v *gocui.View) (string, error) {
	var l string
	var err error

	_, cy := v.Cursor()
	if l, err = v.Line(cy); err != nil {
		l = ""
	}

	return l, nil
}

func (gui *Gui) playNext() error {
	if !queue.isEmpty() {
		gui.playNextFromQueue()
	} else if state.hasPlaylistSelected() {
		gui.playNextFromPlaylist()
	}
	return nil
}

func (gui *Gui) playNextFromPlaylist() {
	playlist := playlists[state.currentPlaylist]
	if state.isAllRandomMode() {
		state.currentPlaylist, state.currentIndexTrack = getRandomNextPlaylistAndTrack()
		playlist = playlists[state.currentPlaylist]
	} else if state.isRandomMode() {
		state.currentIndexTrack = getRandomNextTrack(playlist)
	} else {
		state.currentIndexTrack = getNextTrack(playlist)
	}
	playlistTrack := playlist.Track(state.currentIndexTrack)
	track := playlistTrack.Track()
	track.Wait()

	gui.play(track)
}

func (gui *Gui) playNextFromQueue() {
	gui.play(queue.Pop())
	gui.updateQueueView()
}

func (gui *Gui) play(track *sp.Track) {
	gui.currentTrack = track
	gui.events.Play(gui.currentTrack)
}

func getNextTrack(playlist *sp.Playlist) int {
	if state.currentIndexTrack >= playlist.Tracks()-1 {
		return 0
	}
	return state.currentIndexTrack + 1
}

func getRandomNextTrack(playlist *sp.Playlist) int {
	return rand.Intn(playlist.Tracks())
}

func getRandomNextPlaylistAndTrack() (string, int) {
	index := rand.Intn(len(playlists))
	count := 0
	var playlist *sp.Playlist
	var newPlaylistName string
	for key, value := range playlists {
		if index == count {
			newPlaylistName = key
			playlist = value
			break
		}
		count++
	}
	return newPlaylistName, rand.Intn(playlist.Tracks())
}

func getCurrentSelectedTrack() *sp.Track {
	var errPlaylist error
	state.currentPlaylist, errPlaylist = gui.getSelectedPlaylist()
	currentTrack, errTrack := gui.getSelectedTrack()
	if errPlaylist == nil && errTrack == nil && playlists != nil {
		playlist := playlists[state.currentPlaylist]

		if playlist != nil {
			playlist.Wait()
			currentTrack = currentTrack[0:strings.Index(currentTrack, ".")]
			converted, _ := strconv.Atoi(currentTrack)
			state.currentIndexTrack = converted - 1
			playlistTrack := playlist.Track(state.currentIndexTrack)
			track := playlistTrack.Track()
			track.Wait()
			return track
		}
	}
	return nil
}

func keybindings() error {
	if err := gui.g.SetKeybinding("main", gocui.KeySpace, 0, playCurrentSelectedTrack); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'p', 0, pauseCurrentSelectedTrack); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'r', 0, setRandomMode); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'R', 0, setAllRandomMode); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", '>', 0, nextCommand); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'u', 0, queueCommand); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", gocui.KeyHome, 0, cursorHome); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyEnd, 0, cursorEnd); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", gocui.KeyPgup, 0, cursorPgup); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyPgdn, 0, cursorPgdn); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", gocui.KeyArrowDown, 0, cursorDown); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", gocui.KeyArrowUp, 0, cursorUp); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", gocui.KeyArrowLeft, 0, nextView); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("side", gocui.KeyArrowRight, 0, nextView); err != nil {
		return err
	}

	// vi navigation
	if err := gui.g.SetKeybinding("", 'j', 0, cursorDown); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'k', 0, cursorUp); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("main", 'h', 0, nextView); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("side", 'l', 0, nextView); err != nil {
		return err
	}

	if err := gui.g.SetKeybinding("", gocui.KeyCtrlC, 0, quit); err != nil {
		return err
	}
	if err := gui.g.SetKeybinding("", 'q', 0, quit); err != nil {
		return err
	}

	return nil
}

func (gui *Gui) updateTracksView() {
	gui.tracksView.Clear()
	gui.tracksView.SetCursor(0, 0)
	gui.tracksView.SetOrigin(0, 0)
	currentPlaylist, err := gui.getSelectedPlaylist()
	if err == nil && playlists != nil {
		playlist := playlists[currentPlaylist]

		if playlist != nil {
			playlist.Wait()
			for i := 0; i < playlist.Tracks(); i++ {
				playlistTrack := playlist.Track(i)
				track := playlistTrack.Track()
				track.Wait()
				fmt.Fprintf(gui.tracksView, "%v. %v - %v", (i + 1), track.Artist(0).Name(), track.Name())
			}
		}
	}
}

func (gui *Gui) updatePlaylistsView() {
	gui.playlistsView.Clear()
	if playlists != nil {
		keys := make([]string, len(playlists))
		i := 0
		for k, _ := range playlists {
			keys[i] = k
			i++
		}
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Fprintln(gui.playlistsView, key)
		}
	}
}

func (gui *Gui) updateQueueView() {
	gui.queueView.Clear()
	if !queue.isEmpty() {
		for _, track := range queue.Contents() {
			fmt.Fprintf(gui.queueView, "%v - %v", track.Artist(0).Name(), track.Name())
		}
	}
}

func layout(g *gocui.Gui) error {
	maxX, maxY := g.Size()
	if v, err := g.SetView("side", -1, -1, 25, maxY-2); err != nil {
		if err != gocui.ErrorUnkView {
			return err
		}
		gui.playlistsView = v
		gui.playlistsView.Highlight = true

		gui.updatePlaylistsView()

		if err := g.SetCurrentView("side"); err != nil {
			return err
		}
	}
	if v, err := g.SetView("main", 25, -1, maxX-50, maxY-2); err != nil {
		if err != gocui.ErrorUnkView {
			return err
		}
		gui.tracksView = v

		gui.updateTracksView()
	}
	if v, err := g.SetView("queue", maxX-50, -1, maxX, maxY-2); err != nil {
		if err != gocui.ErrorUnkView {
			return err
		}
		gui.queueView = v
	}
	if v, err := g.SetView("status", -1, maxY-2, maxX, maxY); err != nil {
		if err != gocui.ErrorUnkView {
			return err
		}
		gui.statusView = v
	}
	return nil
}