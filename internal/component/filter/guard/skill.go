package guard

import "github.com/bartdeboer/ctgbot/internal/component"

func (c *Component) Skill() component.Skill {
	return component.Skill{
		Name:        Type,
		Description: "LLM inbound guard filter setup",
		Text: `Guard component

What it does:
- Classifies admitted inbound events before they reach an agent.
- Uses a configured completion engine in restricted JSON mode.
- Passes low-risk messages and quarantines/denies suspicious messages.

Managed files:
- component.json: guard settings. Non-sensitive. Required.

component.json example:
{
  "completion": "llamacpp/qwen-q5"
}

Optional settings:
{
  "max_output_tokens": 512,
  "high_risk_score": 0.70
}

Setup commands:
1. Register the completion engine, for example:
   ctgbot component register llamacpp/qwen-q5 --runtime backend

2. Register this guard profile:
   ctgbot component register guard/qwen --runtime local

3. Install guard config:
   printf '{"completion":"llamacpp/qwen-q5"}' | hostbridge component guard/qwen managed-file put component.json --type application/json

4. Bind the guard as the source filter for a trusted chat/source binding:
   ctgbot chat <chatID> component gmail/personal filter add guard/qwen

Safety notes:
- The guard receives prompt-in/text-out completion access only.
- It does not receive a TurnRuntime, workspace, hostbridge, or command surface.
- Missing or invalid guard output fails closed to quarantine.`,
	}
}
