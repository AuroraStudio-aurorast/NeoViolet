//! Native input events and read-only projections of the Go-owned player state.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Context {
    pub window: iced::window::Id,
    pub session: u64,
    pub epoch: u64,
    pub track_id: Option<u64>,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub enum Command {
    Open,
    PlayPause,
    SeekBy(f64),
    Lyrics,
}

#[derive(Debug, Clone, Copy)]
pub struct Event {
    pub context: Context,
    pub command: Command,
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct State {
    pub context: Context,
    pub available: bool,
    pub can_play: bool,
    pub can_seek: bool,
    pub playing: bool,
    pub lyrics: bool,
}

impl State {
    pub fn allows(self, command: Command) -> bool {
        match command {
            Command::Open | Command::Lyrics => self.available,
            Command::PlayPause => self.can_play,
            Command::SeekBy(offset) => self.can_seek && offset.is_finite(),
        }
    }
}
