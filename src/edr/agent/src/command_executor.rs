use std::process::Command;
use sysinfo::{Pid, System};

#[derive(Debug)]
pub enum AgentCommand {
    KillProcess(u32),
    BlockIp(String),
}

pub struct CommandExecutor;

impl CommandExecutor {
    pub fn new() -> Self {
        Self
    }

    pub fn execute(&self, cmd: AgentCommand) -> Result<(), String> {
        println!("[CommandExecutor] Executing: {:?}", cmd);
        match cmd {
            AgentCommand::KillProcess(pid) => self.kill_process(pid),
            AgentCommand::BlockIp(ip) => self.block_ip(&ip),
        }
    }

    fn kill_process(&self, pid_u32: u32) -> Result<(), String> {
        let mut sys = System::new_all();
        sys.refresh_all();
        let pid = Pid::from_u32(pid_u32);

        if let Some(process) = sys.process(pid) {
            println!("[CommandExecutor] Killing process: {} ({})", process.name(), pid);
            // sysinfo kill usually sends SIGTERM/SIGKILL depending on implementation/signal
            if process.kill() {
                Ok(())
            } else {
                Err(format!("Failed to kill process {}", pid))
            }
        } else {
            Err(format!("Process {} not found", pid))
        }
    }

    fn block_ip(&self, ip: &str) -> Result<(), String> {
        // Validation (basic)
        if ip.chars().filter(|c| *c == '.').count() != 3 {
             return Err(format!("Invalid IP format: {}", ip));
        }

        println!("[CommandExecutor] Blocking IP: {}", ip);
        
        let output = Command::new("iptables")
            .arg("-A")
            .arg("INPUT")
            .arg("-s")
            .arg(ip)
            .arg("-j")
            .arg("DROP")
            .output()
            .map_err(|e| format!("Failed to execute iptables: {}", e))?;

        if output.status.success() {
            Ok(())
        } else {
            let stderr = String::from_utf8_lossy(&output.stderr);
            Err(format!("iptables failed: {}", stderr))
        }
    }
}
