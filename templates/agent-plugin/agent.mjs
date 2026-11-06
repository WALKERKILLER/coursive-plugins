// Plain words: while the mode is on, every turn's system prompt gains an
// instruction to explain concepts plainly. The mode is toggled from the
// composer's "+" menu through the "toggle" action declared in package.json.

const rules = ({ analogy = true } = {}) => `## Plain words mode (turned on by the user)
- Explain each new concept in plain words before using its technical name.
- ${analogy ? "Add one everyday analogy when it helps." : "Do not use analogies."}
- Keep citing lecture timestamps as usual.`;

// Runs once each time the plugin loads: register actions and restore state.
export async function activate(coursive) {
  coursive.status({ active: (await coursive.storage.get("active")) === true });
  coursive.action("toggle", async () => {
    const active = (await coursive.storage.get("active")) !== true;
    await coursive.storage.set("active", active);
    coursive.status({ active });
    return { toast: active ? "Plain words is on" : "Plain words is off", state: { active } };
  });
}

// Runs for every new conversation: register tools and hooks only.
export default function (pi, coursive) {
  pi.on("before_agent_start", async (event) => {
    if ((await coursive.storage.get("active")) !== true) return undefined;
    return { systemPrompt: `${event.systemPrompt}\n\n${rules(coursive.settings())}` };
  });
}
