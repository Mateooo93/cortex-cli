import {
  AbsoluteFill,
  Img,
  Sequence,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";

const BG = "#0B1020";
const CYAN = "#00E5FF";
const BLUE = "#3D8BFF";
const VIOLET = "#7C5BFF";
const TEXT = "#E6EDF3";
const DIM = "#8B9BB4";

const FEATURES = [
  "One binary · no daemon · no Docker",
  "Multi-provider models & subscription OAuth",
  "Goals, sub-agents, todos & project memory",
  "Safe by default — deny lists & confirmations",
];

const INSTALL_CMD =
  "npm install -g @mateooo93/cortex@latest --registry=https://npm.pkg.github.com";

const GradientBg: React.FC<{opacity?: number}> = ({opacity = 1}) => (
  <AbsoluteFill
    style={{
      opacity,
      background: `radial-gradient(ellipse 80% 60% at 50% 40%, ${BLUE}22 0%, transparent 70%), linear-gradient(160deg, ${BG} 0%, #121a32 50%, ${BG} 100%)`,
    }}
  />
);

const Grid: React.FC = () => (
  <AbsoluteFill
    style={{
      opacity: 0.12,
      backgroundImage: `
        linear-gradient(${CYAN}33 1px, transparent 1px),
        linear-gradient(90deg, ${CYAN}33 1px, transparent 1px)
      `,
      backgroundSize: "48px 48px",
    }}
  />
);

const IntroScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const logoSpring = spring({frame, fps, config: {damping: 18, stiffness: 90}});
  const tagSpring = spring({frame: frame - 12, fps, config: {damping: 20}});
  const subSpring = spring({frame: frame - 22, fps, config: {damping: 20}});

  return (
    <AbsoluteFill style={{justifyContent: "center", alignItems: "center"}}>
      <GradientBg />
      <Grid />
      <div style={{textAlign: "center", padding: "0 80px"}}>
        <Img
          src={staticFile("logo.png")}
          style={{
            width: 520,
            opacity: logoSpring,
            transform: `scale(${interpolate(logoSpring, [0, 1], [0.88, 1])})`,
            marginBottom: 36,
          }}
        />
        <div
          style={{
            fontFamily: "system-ui, -apple-system, Segoe UI, sans-serif",
            fontSize: 52,
            fontWeight: 700,
            color: TEXT,
            opacity: tagSpring,
            transform: `translateY(${interpolate(tagSpring, [0, 1], [24, 0])}px)`,
            letterSpacing: -0.5,
          }}
        >
          The open source AI coding agent
        </div>
        <div
          style={{
            marginTop: 14,
            fontFamily: "system-ui, -apple-system, Segoe UI, sans-serif",
            fontSize: 34,
            fontWeight: 500,
            color: CYAN,
            opacity: subSpring,
            transform: `translateY(${interpolate(subSpring, [0, 1], [18, 0])}px)`,
          }}
        >
          for your terminal
        </div>
      </div>
    </AbsoluteFill>
  );
};

const ScreenshotScene: React.FC<{src: string; caption: string}> = ({
  src,
  caption,
}) => {
  const frame = useCurrentFrame();
  const {fps, durationInFrames} = useVideoConfig();

  const enter = spring({frame, fps, config: {damping: 22}});
  const drift = interpolate(frame, [0, durationInFrames], [0, -12]);
  const scale = interpolate(enter, [0, 1], [0.94, 1]);

  return (
    <AbsoluteFill style={{justifyContent: "center", alignItems: "center"}}>
      <GradientBg />
      <Grid />
      <div
        style={{
          width: "88%",
          opacity: enter,
          transform: `translateY(${drift}px) scale(${scale})`,
        }}
      >
        <div
          style={{
            borderRadius: 14,
            overflow: "hidden",
            boxShadow: `0 24px 80px ${BLUE}44, 0 0 0 1px ${CYAN}33`,
          }}
        >
          <Img src={staticFile(src)} style={{width: "100%", display: "block"}} />
        </div>
        <div
          style={{
            marginTop: 28,
            textAlign: "center",
            fontFamily: "system-ui, sans-serif",
            fontSize: 30,
            color: DIM,
            fontStyle: "italic",
          }}
        >
          {caption}
        </div>
      </div>
    </AbsoluteFill>
  );
};

const FeaturesScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  return (
    <AbsoluteFill style={{justifyContent: "center", alignItems: "center"}}>
      <GradientBg />
      <Grid />
      <div style={{width: "78%"}}>
        <div
          style={{
            fontFamily: "system-ui, sans-serif",
            fontSize: 44,
            fontWeight: 700,
            color: TEXT,
            marginBottom: 36,
            textAlign: "center",
          }}
        >
          Built for daily development
        </div>
        {FEATURES.map((line, i) => {
          const delay = i * 10;
          const prog = spring({
            frame: frame - delay,
            fps,
            config: {damping: 18},
          });
          return (
            <div
              key={line}
              style={{
                display: "flex",
                alignItems: "center",
                gap: 18,
                marginBottom: 22,
                opacity: prog,
                transform: `translateX(${interpolate(prog, [0, 1], [-40, 0])}px)`,
              }}
            >
              <div
                style={{
                  width: 12,
                  height: 12,
                  borderRadius: "50%",
                  background: `linear-gradient(135deg, ${CYAN}, ${VIOLET})`,
                  flexShrink: 0,
                }}
              />
              <div
                style={{
                  fontFamily: "system-ui, sans-serif",
                  fontSize: 30,
                  color: TEXT,
                }}
              >
                {line}
              </div>
            </div>
          );
        })}
      </div>
    </AbsoluteFill>
  );
};

const InstallScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const titleSpring = spring({frame, fps});
  const boxSpring = spring({frame: frame - 10, fps, config: {damping: 20}});
  const typeStart = 18;
  const typeEnd = typeStart + 72;
  const chars = Math.floor(
    interpolate(frame, [typeStart, typeEnd], [0, INSTALL_CMD.length], {
      extrapolateLeft: "clamp",
      extrapolateRight: "clamp",
    }),
  );
  const typingDone = chars >= INSTALL_CMD.length;

  return (
    <AbsoluteFill style={{justifyContent: "center", alignItems: "center"}}>
      <GradientBg />
      <Grid />
      <div style={{width: "82%", textAlign: "center"}}>
        <div
          style={{
            fontFamily: "system-ui, sans-serif",
            fontSize: 44,
            fontWeight: 700,
            color: TEXT,
            opacity: titleSpring,
            marginBottom: 32,
          }}
        >
          Get started in one command
        </div>
        <div
          style={{
            opacity: boxSpring,
            transform: `scale(${interpolate(boxSpring, [0, 1], [0.96, 1])})`,
            borderRadius: 12,
            padding: "28px 36px",
            background: "#161B22",
            border: `1px solid ${CYAN}44`,
            boxShadow: `0 16px 48px ${BLUE}33`,
          }}
        >
          <div
            style={{
              fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
              fontSize: 28,
              color: CYAN,
              textAlign: "left",
              whiteSpace: "pre-wrap",
              wordBreak: "break-all",
            }}
          >
            {INSTALL_CMD.slice(0, chars)}
            <span style={{opacity: frame % 20 < 10 ? 1 : 0, color: TEXT}}>|</span>
          </div>
          <div
            style={{
              marginTop: 18,
              fontFamily: "ui-monospace, monospace",
              fontSize: 28,
              color: VIOLET,
              textAlign: "left",
              opacity: typingDone
                ? spring({frame: frame - typeEnd - 4, fps, config: {damping: 20}})
                : 0,
            }}
          >
            cortex
          </div>
        </div>
        <div
          style={{
            marginTop: 28,
            fontFamily: "system-ui, sans-serif",
            fontSize: 24,
            color: DIM,
            opacity: spring({frame: frame - 30, fps}),
          }}
        >
          macOS · Linux · Windows
        </div>
      </div>
    </AbsoluteFill>
  );
};

const OutroScene: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const enter = spring({frame, fps, config: {damping: 16}});

  return (
    <AbsoluteFill style={{justifyContent: "center", alignItems: "center"}}>
      <GradientBg />
      <Grid />
      <div style={{textAlign: "center", opacity: enter}}>
        <Img
          src={staticFile("logo.png")}
          style={{
            width: 420,
            transform: `scale(${interpolate(enter, [0, 1], [0.9, 1])})`,
            marginBottom: 28,
          }}
        />
        <div
          style={{
            fontFamily: "system-ui, sans-serif",
            fontSize: 36,
            fontWeight: 600,
            color: TEXT,
          }}
        >
          github.com/Mateooo93/cortex-cli
        </div>
        <div
          style={{
            marginTop: 16,
            fontFamily: "system-ui, sans-serif",
            fontSize: 26,
            color: CYAN,
          }}
        >
          One binary. Beautiful TUI. Your models, your machine.
        </div>
      </div>
    </AbsoluteFill>
  );
};

export const CortexPromo: React.FC = () => {
  return (
    <AbsoluteFill style={{backgroundColor: BG}}>
      <Sequence durationInFrames={90}>
        <IntroScene />
      </Sequence>
      <Sequence from={75} durationInFrames={105}>
        <ScreenshotScene
          src="welcome.png"
          caption="Polished Bubble Tea interface — sessions, chat, and settings"
        />
      </Sequence>
      <Sequence from={165} durationInFrames={120}>
        <ScreenshotScene
          src="demo.png"
          caption="Plans, tool calls, file edits, and live todos"
        />
      </Sequence>
      <Sequence from={270} durationInFrames={105}>
        <FeaturesScene />
      </Sequence>
      <Sequence from={360} durationInFrames={105}>
        <InstallScene />
      </Sequence>
      <Sequence from={450} durationInFrames={90}>
        <OutroScene />
      </Sequence>
    </AbsoluteFill>
  );
};