import { ChildProcess, spawn, execSync } from 'child_process';
import * as path from 'path';
import * as fs from 'fs';
import * as os from 'os';

export interface ServerOptions {
  port?: number;
  /** Extra environment variables to pass to the server process */
  env?: Record<string, string>;
}

/**
 * Manages a DashGate server process for E2E testing.
 * Used by OIDC tests that need their own isolated server instance.
 * Main tests use Playwright's webServer config instead.
 */
export class DashboardServer {
  private proc: ChildProcess | null = null;
  private tempDir: string = '';
  private _port: number;
  private _baseURL: string;

  constructor(private opts: ServerOptions = {}) {
    this._port = opts.port || 11738;
    this._baseURL = `http://localhost:${this._port}`;
  }

  get port(): number {
    return this._port;
  }

  get baseURL(): string {
    return this._baseURL;
  }

  get dbPath(): string {
    return path.join(this.tempDir, 'dashgate.db');
  }

  async start(): Promise<void> {
    this.tempDir = fs.mkdtempSync(path.join(os.tmpdir(), 'dashgate-e2e-'));

    const projectRoot = path.resolve(__dirname, '..', '..');
    const executableName = process.platform === 'win32' ? 'dashgate.exe' : 'dashgate';
    const executablePath = path.join(projectRoot, executableName);

    if (!fs.existsSync(executablePath)) {
      throw new Error(
        `DashGate executable not found at ${executablePath}. Run build first.`
      );
    }

    const configPath = path.join(__dirname, 'test-config.yaml');
    const templatesPath = path.join(projectRoot, 'templates');
    const staticPath = path.join(projectRoot, 'static');
    const iconsPath = path.join(projectRoot, 'static', 'icons');

    const env: Record<string, string> = {
      ...process.env as Record<string, string>,
      CONFIG_PATH: configPath,
      DB_PATH: this.dbPath,
      ICONS_PATH: iconsPath,
      PORT: String(this._port),
      COOKIE_SECURE: 'false',
      TEMPLATES_PATH: templatesPath,
      STATIC_PATH: staticPath,
      DEV_MODE: 'false',
      ...this.opts.env,
    };

    this.proc = spawn(executablePath, [], {
      env,
      stdio: ['ignore', 'pipe', 'pipe'],
      cwd: projectRoot,
      windowsHide: true,
    });

    this.proc.stdout?.on('data', (data: Buffer) => {
      if (process.env.DEBUG) {
        process.stdout.write(`[dashgate:${this._port}] ${data}`);
      }
    });
    this.proc.stderr?.on('data', (data: Buffer) => {
      if (process.env.DEBUG) {
        process.stderr.write(`[dashgate:${this._port}:err] ${data}`);
      }
    });

    this.proc.on('error', (err: Error) => {
      console.error(`DashGate server error (port ${this._port}):`, err);
    });

    this.proc.on('exit', (code: number | null, signal: string | null) => {
      if (process.env.DEBUG) {
        console.log(`DashGate (port ${this._port}) exited: code=${code}, signal=${signal}`);
      }
    });

    await this.waitForHealth();
  }

  private async waitForHealth(timeoutMs = 15_000): Promise<void> {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      try {
        const response = await fetch(`${this._baseURL}/health`);
        if (response.ok) {
          return;
        }
      } catch {
        // Server not ready yet
      }
      await new Promise((r) => setTimeout(r, 250));
    }
    throw new Error(`DashGate did not become healthy within ${timeoutMs}ms`);
  }

  async stop(): Promise<void> {
    if (this.proc && this.proc.pid) {
      const pid = this.proc.pid;
      try {
        if (process.platform === 'win32') {
          // On Windows, use taskkill to kill the process tree
          execSync(`taskkill /pid ${pid} /T /F`, { stdio: 'ignore' });
        } else {
          this.proc.kill('SIGTERM');
        }
      } catch {
        // Process may already be dead
      }

      // Wait for exit
      await new Promise<void>((resolve) => {
        const timer = setTimeout(() => {
          try {
            this.proc?.kill('SIGKILL');
          } catch { /* already dead */ }
          resolve();
        }, 3000);
        this.proc!.on('exit', () => {
          clearTimeout(timer);
          resolve();
        });
      });
      this.proc = null;
    }

    // Clean up temp directory
    if (this.tempDir && fs.existsSync(this.tempDir)) {
      try {
        fs.rmSync(this.tempDir, { recursive: true, force: true });
      } catch {
        // Best effort cleanup
      }
    }
  }
}
