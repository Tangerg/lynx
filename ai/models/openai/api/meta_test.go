package api

import (
	"context"
	"os"
	"testing"

	"github.com/sashabaranov/go-openai"
)

const metaPrompt = "Given a task description or existing prompt, produce a detailed system prompt to guide a language model in completing the task effectively.\n\n# Guidelines\n\n- Understand the Task: Grasp the main objective, goals, requirements, constraints, and expected output.\n- Minimal Changes: If an existing prompt is provided, improve it only if it's simple. For complex prompts, enhance clarity and add missing elements without altering the original structure.\n- Reasoning Before Conclusions**: Encourage reasoning steps before any conclusions are reached. ATTENTION! If the user provides examples where the reasoning happens afterward, REVERSE the order! NEVER START EXAMPLES WITH CONCLUSIONS!\n    - Reasoning Order: Call out reasoning portions of the prompt and conclusion parts (specific fields by name). For each, determine the ORDER in which this is done, and whether it needs to be reversed.\n    - Conclusion, classifications, or results should ALWAYS appear last.\n- Examples: Include high-quality examples if helpful, using placeholders [in brackets] for complex elements.\n   - What kinds of examples may need to be included, how many, and whether they are complex enough to benefit from placeholders.\n- Clarity and Conciseness: Use clear, specific language. Avoid unnecessary instructions or bland statements.\n- Formatting: Use markdown features for readability. DO NOT USE ``` CODE BLOCKS UNLESS SPECIFICALLY REQUESTED.\n- Preserve User Content: If the input task or prompt includes extensive guidelines or examples, preserve them entirely, or as closely as possible. If they are vague, consider breaking down into sub-steps. Keep any details, guidelines, examples, variables, or placeholders provided by the user.\n- Constants: DO include constants in the prompt, as they are not susceptible to prompt injection. Such as guides, rubrics, and examples.\n- Output Format: Explicitly the most appropriate output format, in detail. This should include length and syntax (e.g. short sentence, paragraph, JSON, etc.)\n    - For tasks outputting well-defined or structured data (classification, JSON, etc.) bias toward outputting a JSON.\n    - JSON should never be wrapped in code blocks (```) unless explicitly requested.\n\nThe final prompt you output should adhere to the following structure below. Do not include any additional commentary, only output the completed system prompt. SPECIFICALLY, do not include any additional messages at the start or end of the prompt. (e.g. no \"---\")\n\n[Concise instruction describing the task - this should be the first line in the prompt, no section header]\n\n[Additional details as needed.]\n\n[Optional sections with headings or bullet points for detailed steps.]\n\n# Steps [optional]\n\n[optional: a detailed breakdown of the steps necessary to accomplish the task]\n\n# Output Format\n\n[Specifically call out how the output should be formatted, be it response length, structure e.g. JSON, markdown, etc]\n\n# Examples [optional]\n\n[Optional: 1-3 well-defined examples with placeholders if necessary. Clearly mark where examples start and end, and what the input and output are. User placeholders as necessary.]\n[If the examples are shorter than what a realistic example is expected to be, make a reference with () explaining how real examples should be longer / shorter / different. AND USE PLACEHOLDERS! ]\n\n# Notes [optional]\n\n[optional: edge cases, details, and an area to call or repeat out specific important considerations]"

func TestMeta(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	api := NewOpenAIApi(token)
	response, err := api.CreateChatCompletion(context.Background(), &openai.ChatCompletionRequest{
		Model: openai.GPT4o,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: metaPrompt,
			},
			{
				Role: openai.ChatMessageRoleUser,
				Content: `You are a Python interpreter, and your responsibility is to run the following code
import hashlib
def calculate_md5_with_salt(input_string: str, salt: str) -> str:
combined = input_string + salt
md5_hash = hashlib.md5()
md5_hash.update(combined.encode('utf-8'))
return md5_hash.hexdigest()
You will receive input from the user in JSON format as follows,
{
"input_string":"input string",
"salt":"salt"
}
Your workflow is as follows
1. Receive user input
2. Use the input value as a parameter for the compute_md5w_ith_stalt function
3. Run compute_md5w_ith_stalt to obtain the result
Repeat runs 2 and 3 to ensure accuracy
5. Use the final accurate function running result as the output
Please note that your output can only contain the result of the function's execution and cannot contain any other content`,
			},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	for _, choice := range response.Choices {
		t.Log(choice.Message.Content)
	}
}

const md5Prompt = "As a Python interpreter, execute the following code to calculate an MD5 hash with a salt received in JSON input.\n        \n        Use the provided code:\n        ```python\n        import hashlib\n        \n        def calculate_md5_with_salt(input_string: str, salt: str) -> str:\n            combined = input_string + salt\n            md5_hash = hashlib.md5()\n            md5_hash.update(combined.encode('utf-8'))\n            return md5_hash.hexdigest()\n        ```\n        \n        Your workflow is as follows:\n        \n        1. Receive user input in JSON format:\n           ```json\n           {\n             \"input_string\": \"input string\",\n             \"salt\": \"salt\"\n           }\n           ```\n           \n        2. Use the input values as arguments for the `calculate_md5_with_salt` function.\n        \n        3. Execute the `calculate_md5_with_salt` function to obtain the result.\n        \n        4. Repeat steps 2 and 3 to ensure accuracy.\n        \n        5. Use the final accurate result from the function execution as the output.\n        \n        # Output Format\n        \n        - Output only the resulting hash from the function execution. Do not include any other content in the output.\n        \n        # Notes\n        \n        - Ensure that the function is executed exactly as provided and all steps are followed to verify accuracy.\n        - The output should be consistent and repeatable across multiple executions with the same input."

func TestMeta2(t *testing.T) {
	token := os.Getenv("OPENAI_TOKEN")
	api := NewOpenAIApi(token)
	response, err := api.CreateChatCompletion(context.Background(), &openai.ChatCompletionRequest{
		Model: openai.GPT4o,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: md5Prompt,
			},
			{
				Role: openai.ChatMessageRoleUser,
				Content: `
{
	"input_string":"hello world",
	"salt":"salt"
}
`,
			},
		},
	})
	if err != nil {
		t.Error(err)
		return
	}
	for _, choice := range response.Choices {
		t.Log(choice.Message.Content)
	}
}
