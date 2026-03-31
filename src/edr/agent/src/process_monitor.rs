use sysinfo::{Pid, System};
use std::collections::HashSet;
use std::time::Duration;
use tokio::time::sleep;
use reqwest::Client;
use serde::Serialize;

#[derive(Serialize, Debug)]
pub struct ProcessEntry {
    pub hostname: String,
    pub pid: u32,
    pub name: String,
    pub cmd: String,
}

pub struct ProcessMonitor {
    known_pids: HashSet<Pid>,
    system: System,
    client: Client,
    base_url: String,
}

impl ProcessMonitor {
    pub fn new(client: Client, base_url: &str) -> Self {
        let mut system = System::new_all();
        system.refresh_all();

        // Initial snapshot
        let known_pids: HashSet<Pid> = system.processes().keys().cloned().collect();
        println!("[ProcessMonitor] Initialized with {} processes.", known_pids.len());

        Self {
            known_pids,
            system,
            client,
            base_url: base_url.to_string(),
        }
    }

    pub async fn run(mut self) {
        println!("[ProcessMonitor] Starting poller...");
        let processes_url = format!("{}/processes", self.base_url);

        loop {
            // Wait before next scan
            sleep(Duration::from_secs(2)).await;

            // Refresh processes
            self.system.refresh_processes();

            // Detect new PIDs
            let current_pids: HashSet<Pid> = self.system.processes().keys().cloned().collect();

            for pid in &current_pids {
                if !self.known_pids.contains(pid) {
                    if let Some(process) = self.system.process(*pid) {
                        let name = process.name().to_string();
                        let cmd = process.cmd().join(" ");

                        println!(
                            "[ProcessMonitor] NEW PROCESS: PID={} Name={} Cmd={}",
                            pid,
                            name,
                            cmd,
                        );

                        let entry = ProcessEntry {
                            hostname: sysinfo::System::host_name().unwrap_or_else(|| "unknown".to_string()),
                            pid: pid.as_u32(),
                            name: name.clone(),
                            cmd: cmd.clone(),
                        };

                        // Fire-and-forget POST to backend
                        let client = self.client.clone();
                        let url = processes_url.clone();
                        tokio::spawn(async move {
                            match client.post(&url).json(&entry).send().await {
                                Ok(resp) => {
                                    if !resp.status().is_success() {
                                        eprintln!("[ProcessMonitor] Backend returned: {}", resp.status());
                                    }
                                }
                                Err(e) => {
                                    eprintln!("[ProcessMonitor] Failed to POST process: {}", e);
                                }
                            }
                        });
                    }
                }
            }

            // Update state (forget old PIDs, keep current)
            self.known_pids = current_pids;
        }
    }
}
