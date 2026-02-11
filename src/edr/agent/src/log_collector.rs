use std::fs::File;
use std::io::{BufRead, BufReader, Seek, SeekFrom};
use std::path::Path;
use std::time::Duration;
use tokio::time::sleep;

pub struct LogCollector {
    file_path: String,
}

impl LogCollector {
    pub fn new(file_path: &str) -> Self {
        Self {
            file_path: file_path.to_string(),
        }
    }

    pub async fn run(&self) {
        println!("[LogCollector] Starting watch on: {}", self.file_path);

        let mut file = match File::open(&self.file_path) {
            Ok(f) => f,
            Err(e) => {
                eprintln!("[LogCollector] Error opening file {}: {}", self.file_path, e);
                return;
            }
        };

        // Move to the end of the file to read only new logs
        if let Err(e) = file.seek(SeekFrom::End(0)) {
            eprintln!("[LogCollector] Error seeking file: {}", e);
            return;
        }

        let mut reader = BufReader::new(file);
        let mut line = String::new();

        loop {
            line.clear();
            match reader.read_line(&mut line) {
                Ok(0) => {
                    // EOF, wait a bit
                    sleep(Duration::from_millis(500)).await;
                }
                Ok(_) => {
                    // Print the new line (trim to remove newline at the end)
                    if !line.trim().is_empty() {
                        println!("[LogCollector] NEW LOG: {}", line.trim());
                    }
                }
                Err(e) => {
                    eprintln!("[LogCollector] Error reading line: {}", e);
                    sleep(Duration::from_secs(1)).await;
                }
            }
        }
    }
}
