use sysinfo::{Pid, System};
use std::collections::HashSet;
use std::time::Duration;
use tokio::time::sleep;

pub struct ProcessMonitor {
    known_pids: HashSet<Pid>,
    system: System,
}

impl ProcessMonitor {
    pub fn new() -> Self {
        let mut system = System::new_all();
        system.refresh_all();
        
        // Initial snapshot
        let known_pids: HashSet<Pid> = system.processes().keys().cloned().collect();
        println!("[ProcessMonitor] Initialized with {} processes.", known_pids.len());

        Self {
            known_pids,
            system,
        }
    }

    pub async fn run(mut self) {
        println!("[ProcessMonitor] Starting poller...");
        
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
                        println!(
                            "[ProcessMonitor] NEW PROCESS: PID={} Name={} Cmd={:?}", 
                            pid, 
                            process.name(), 
                            process.cmd()
                        );
                    }
                }
            }

            // Update state (forget old PIDs, keep current)
            self.known_pids = current_pids;
        }
    }
}
