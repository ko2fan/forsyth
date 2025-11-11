package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/ssh"
	"github.com/charmbracelet/wish"
	"github.com/charmbracelet/wish/activeterm"
	"github.com/charmbracelet/wish/bubbletea"
	"github.com/charmbracelet/wish/logging"
)

const (
	host = "localhost"
	port = "23234"
)

var admins = map[string]string{
	"david": "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIG7R7d3cs3fORreSEmoXdaiiLELGxatOn0nNR7UGMxTB",
}

func main() {
	srv, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519_server"),
		wish.WithPublicKeyAuth(func(ctx ssh.Context, key ssh.PublicKey) bool {
			return true
		}),
		ssh.AllocatePty(),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
			activeterm.Middleware(),
			// The last item in the chain is the first to be called.
			logging.Middleware(),
		),
	)
	if err != nil {
		log.Error("Could not start server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Info("Starting SSH server", "host", host, "port", port)
		if err = srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			// Ignore ErrServerClosed because it is expected.
			log.Error("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() { cancel() }()

	log.Info("Stopping SSH server")
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Could not stop server", "error", err)
	}
}

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	// This should never fail, as we are using the activeterm middleware.
	pty, _, _ := s.Pty()

	// When running a Bubble Tea app over SSH, you shouldn't use the default
	// lipgloss.NewStyle function.
	// That function will use the color profile from the os.Stdin, which is the
	// server, not the client.
	// We provide a MakeRenderer function in the bubbletea middleware package,
	// so you can easily get the correct renderer for the current session, and
	// use it to create the styles.
	// The recommended way to use these styles is to then pass them down to
	// your Bubble Tea model.
	renderer := bubbletea.MakeRenderer(s)
	txtStyle := renderer.NewStyle().Foreground(lipgloss.Color("10"))
	quitStyle := renderer.NewStyle().Foreground(lipgloss.Color("8"))

	bg := "light"
	if renderer.HasDarkBackground() {
		bg = "dark"
	}
	text := textinput.New()
	text.Prompt = "> "
	text.Focus()
	text.CharLimit = 32
	text.Width = 32

	m := model{
		term:      pty.Term,
		profile:   renderer.ColorProfile().Name(),
		width:     pty.Window.Width,
		height:    pty.Window.Height,
		bg:        bg,
		txtStyle:  txtStyle,
		quitStyle: quitStyle,
		name:      s.User(),
		location:  "",
		textInput: text,
	}
	return m, []tea.ProgramOption{tea.WithAltScreen()}
}

type model struct {
	term      string
	profile   string
	width     int
	height    int
	bg        string
	txtStyle  lipgloss.Style
	quitStyle lipgloss.Style
	name      string
	location  string
	textInput textinput.Model
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.textInput.Value() == "quit" {
				return m, tea.Quit
			} else {
				m.textInput.Reset()
			}
			return m, nil
		}
	}
	m.textInput, cmd = m.textInput.Update(msg)

	return m, cmd
}

func (m model) View() string {
	var output string
	if m.location == "north" {
		output = "You see a mountain"
	} else {
		output = fmt.Sprintf("Hello %s. Welcome to Forsyth a text adventure game.", m.name)
	}

	return m.txtStyle.Render(output) + "\n\n" + m.textInput.View() + m.quitStyle.Render("Press 'q' to quit\n")
}
