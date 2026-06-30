# cortex-cli

so basically its an ai coding agent for your terminal. you type what you want and it reads your project, edits files, runs commands, that kind of stuff. one binary, no docker or weird setup. you just need an api key (openai, anthropic, ollama, etc).

## install

**npm** (mac, linux, windows):

```bash
npm install -g mateooo93-cortex@latest
cortex
```

**curl** (mac / linux):

```bash
curl -fsSL https://raw.githubusercontent.com/Mateooo93/cortex-cli/main/script/install.sh | bash
```

**windows** (powershell):

```powershell
irm https://raw.githubusercontent.com/Mateooo93/cortex-cli/main/script/install.ps1 | iex
```

**linux** (download binary):

```bash
curl -fsSL -o cortex https://github.com/Mateooo93/cortex-cli/releases/latest/download/cortex-linux-amd64
chmod +x cortex && mv cortex ~/.local/bin/
```

if npm says permission denied:

```bash
mkdir -p ~/.npm-global
npm config set prefix ~/.npm-global
echo 'export PATH="$HOME/.npm-global/bin:$PATH"' >> ~/.profile
. ~/.profile
npm install -g mateooo93-cortex@latest
```

demo: https://mateooo93.github.io/cortex-cli/

license: AGPL-3.0
