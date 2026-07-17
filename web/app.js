const log = document.getElementById("log");
const composer = document.getElementById("composer");
const input = document.getElementById("messageInput");
const portrait = document.getElementById("portrait");
const portraitImg = document.getElementById("portraitImg");
const audioPlayer = document.getElementById("audioPlayer");
const voiceToggle = document.getElementById("voiceToggle");
const statusDot = document.getElementById("statusDot");

const EXPRESSION_IMAGES = {
  normal: "/img/char_normal.svg",
  happy: "/img/char_happy.svg",
  sad: "/img/char_sad.svg",
  tired: "/img/char_tired.svg",
};

function addBubble(text, kind) {
  const el = document.createElement("div");
  el.className = `bubble ${kind}`;
  el.textContent = text;
  log.appendChild(el);
  log.scrollTop = log.scrollHeight;
  return el;
}

function setExpression(expression) {
  const src = EXPRESSION_IMAGES[expression] || EXPRESSION_IMAGES.normal;
  portraitImg.src = src;
}

async function sendMessage(message) {
  addBubble(message, "user");
  input.value = "";
  input.disabled = true;

  const thinking = addBubble("……", "thinking");

  try {
    const res = await fetch("/api/chat", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ message }),
    });
    if (!res.ok) throw new Error(`chat api error: ${res.status}`);
    const data = await res.json();

    thinking.remove();
    addBubble(data.reply, "partner");
    setExpression(data.expression);
    statusDot.style.background = "";

    if (voiceToggle.checked) {
      playVoice(data.reply);
    }
  } catch (err) {
    thinking.remove();
    addBubble("（応答の取得に失敗しました。サーバーが起動しているか確認してください）", "thinking");
    console.error(err);
  } finally {
    input.disabled = false;
    input.focus();
  }
}

async function playVoice(text) {
  try {
    const res = await fetch("/api/speak", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ text }),
    });
    if (!res.ok) {
      // VOICEVOXが起動していない等。音声なしで続行する。
      statusDot.style.background = "#8A6A7A";
      statusDot.title = "VOICEVOXに接続できませんでした";
      return;
    }
    statusDot.style.background = "";
    statusDot.title = "";

    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    audioPlayer.src = url;

    portrait.classList.add("speaking");
    audioPlayer.onended = () => {
      portrait.classList.remove("speaking");
      URL.revokeObjectURL(url);
    };
    await audioPlayer.play();
  } catch (err) {
    portrait.classList.remove("speaking");
    console.error("音声再生に失敗しました", err);
  }
}

composer.addEventListener("submit", (e) => {
  e.preventDefault();
  const message = input.value.trim();
  if (!message) return;
  sendMessage(message);
});

addBubble("こんにちは。何か話しかけてみてください。", "partner");
