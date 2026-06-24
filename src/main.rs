//! focci — refocus your terminal/editor when an AI coding agent finishes
//! a turn or needs your attention.
//!
//! Invoked as a hook by the agent. The `claude` and `codex` subcommands are the
//! hook entry points; `install`/`uninstall`/`doctor`/`focus` are for the user.
//! The hook entry points always exit 0 — focci is purely observational
//! and must never block or fail an agent's turn.

mod doctor;
mod event;
mod focus;
mod install;

use std::io::{IsTerminal, Read};

use clap::{Parser, Subcommand, ValueEnum};

use event::{ClaudeEvent, Decision};

#[derive(Parser)]
#[command(
    name = "focci",
    version,
    about = "Refocus your terminal/editor when an AI coding agent needs your attention.",
    propagate_version = true
)]
struct Cli {
    #[command(subcommand)]
    command: Command,
}

#[derive(Subcommand)]
enum Command {
    /// Handle a Claude Code hook event (reads the hook JSON on stdin).
    Claude {
        /// Event hint, used only when stdin lacks `hook_event_name`.
        #[arg(long, value_enum)]
        event: Option<ClaudeEventArg>,
    },
    /// Handle a Codex notify event (reads JSON from the last argument, else stdin).
    Codex {
        /// The JSON payload Codex appends as the final `notify` argument.
        payload: Option<String>,
    },
    /// Bring the host terminal/editor to the front now (manual trigger / test).
    Focus {
        /// Ignore the debounce window.
        #[arg(long)]
        force: bool,
    },
    /// Wire focci into your agents' configuration files.
    Install {
        /// Which agent(s) to configure.
        #[arg(long, value_enum, default_value_t = AgentArg::All)]
        agent: AgentArg,
        /// Override the command written into configs (default: this binary's path).
        #[arg(long)]
        command: Option<String>,
        /// For Codex: overwrite an existing unrelated `notify` setting.
        #[arg(long)]
        force: bool,
    },
    /// Remove focci from your agents' configuration files.
    Uninstall {
        /// Which agent(s) to clean up.
        #[arg(long, value_enum, default_value_t = AgentArg::All)]
        agent: AgentArg,
    },
    /// Print detected configuration and integration status.
    Doctor,
}

#[derive(Copy, Clone, ValueEnum)]
enum ClaudeEventArg {
    Stop,
    Notification,
}

impl From<ClaudeEventArg> for ClaudeEvent {
    fn from(arg: ClaudeEventArg) -> Self {
        match arg {
            ClaudeEventArg::Stop => ClaudeEvent::Stop,
            ClaudeEventArg::Notification => ClaudeEvent::Notification,
        }
    }
}

#[derive(Copy, Clone, PartialEq, Eq, ValueEnum)]
enum AgentArg {
    Claude,
    Codex,
    All,
}

impl AgentArg {
    fn includes_claude(self) -> bool {
        matches!(self, AgentArg::Claude | AgentArg::All)
    }
    fn includes_codex(self) -> bool {
        matches!(self, AgentArg::Codex | AgentArg::All)
    }
}

fn debug(message: &str) {
    let enabled = std::env::var("FOCCI_DEBUG")
        .map(|value| !value.is_empty() && value != "0")
        .unwrap_or(false);
    if enabled {
        eprintln!("[focci] {message}");
    }
}

/// Read stdin to a string, unless it's a terminal (manual invocation), in which
/// case there's nothing piped and we must not block.
fn read_stdin() -> String {
    let mut stdin = std::io::stdin();
    if stdin.is_terminal() {
        return String::new();
    }
    let mut buffer = String::new();
    let _ = stdin.read_to_string(&mut buffer);
    buffer
}

fn apply_decision(decision: Decision) {
    match decision {
        Decision::Refocus(reason) => {
            debug(&format!("refocus: {reason}"));
            let outcome = focus::refocus(false);
            debug(&format!("outcome: {outcome:?}"));
        }
        Decision::Ignore(reason) => debug(&format!("ignore: {reason}")),
    }
}

fn run_focus(force: bool) -> ! {
    match focus::refocus(force) {
        focus::FocusOutcome::Activated(bundle) => println!("Focused {bundle}"),
        focus::FocusOutcome::Debounced(bundle) => {
            println!("Skipped (debounced) {bundle} — use --force to override")
        }
        focus::FocusOutcome::NoTarget => {
            eprintln!(
                "No host app detected (no __CFBundleIdentifier or known TERM_PROGRAM). \
                 Set FOCCI_BUNDLE_ID to your terminal's bundle id."
            );
            std::process::exit(1);
        }
        focus::FocusOutcome::Failed(info) => {
            eprintln!("Failed to focus {info}");
            std::process::exit(1);
        }
    }
    std::process::exit(0);
}

fn run_install(agent: AgentArg, command: Option<String>, force: bool) -> ! {
    let mut failed = false;
    if agent.includes_claude() {
        match install::install_claude(command.as_deref()) {
            Ok(report) => println!("{}", report.describe_install("Claude Code")),
            Err(err) => {
                eprintln!("Claude Code install failed: {err}");
                failed = true;
            }
        }
    }
    if agent.includes_codex() {
        match install::install_codex(command.as_deref(), force) {
            Ok(report) => println!("{}", report.describe_install("Codex")),
            Err(err) => {
                eprintln!("Codex install failed: {err}");
                failed = true;
            }
        }
    }
    std::process::exit(if failed { 1 } else { 0 });
}

fn run_uninstall(agent: AgentArg) -> ! {
    let mut failed = false;
    if agent.includes_claude() {
        match install::uninstall_claude() {
            Ok(report) => println!("{}", report.describe_uninstall("Claude Code")),
            Err(err) => {
                eprintln!("Claude Code uninstall failed: {err}");
                failed = true;
            }
        }
    }
    if agent.includes_codex() {
        match install::uninstall_codex() {
            Ok(report) => println!("{}", report.describe_uninstall("Codex")),
            Err(err) => {
                eprintln!("Codex uninstall failed: {err}");
                failed = true;
            }
        }
    }
    std::process::exit(if failed { 1 } else { 0 });
}

fn main() {
    let cli = Cli::parse();
    match cli.command {
        Command::Claude { event } => {
            let stdin = read_stdin();
            apply_decision(event::decide_claude(event.map(Into::into), &stdin));
            std::process::exit(0);
        }
        Command::Codex { payload } => {
            let json = payload.unwrap_or_else(read_stdin);
            apply_decision(event::decide_codex(&json));
            std::process::exit(0);
        }
        Command::Focus { force } => run_focus(force),
        Command::Install {
            agent,
            command,
            force,
        } => run_install(agent, command, force),
        Command::Uninstall { agent } => run_uninstall(agent),
        Command::Doctor => {
            doctor::run();
            std::process::exit(0);
        }
    }
}
