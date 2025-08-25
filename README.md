# microchat.ai

**For having ultra-compressed chats with an LLM to save bandwidth.**

I am frequently on a plane with a 30MB in-flight Wi-Fi plan, and I want to have a
conversation with a SOTA LLM like Claude or Gemini.

The problem is that standard chat clients burn through data. Their requests are
verbose and the text isn't compressed, so my 30MB data cap disappears in
minutes.

So how can I have a long, useful conversation with a SOTA LLM without
running out of data?

I built `microchat.ai`, a system that uses a compression proxy to solve this
problem. It works using two parts:

1. **A lightweight client:** The terminal app on your laptop that you use to
    chat and that measures your data usage.
2. **A proxy server:** An intermediary server that takes your message,
    compresses it, talks to the LLM, and then compresses the response before
    sending it back.

This architecture strips out all the protocol overhead and focuses on
transferring only the essential, compressed data, letting you chat for hours,
not minutes.

## Privacy

I built this with privacy in mind.

* **No User Tracking:** The system uses no authentication or user tracking.
* **Ephemeral Sessions:** Your conversation is tied to an anonymous session ID.
    All server-side history is discarded when the session ends.
* **No Logs:** The proxy server keeps no logs of your conversations.

**A Note on LLM Providers:**

While `microchat.ai` is designed to be private, your questions are ultimately sent
to an LLM provider (like Anthropic, OpenAI, Gemini, etc.). These providers have
their own data policies and may log your conversations.

Never send passwords, API keys, or any other sensitive information.

## Usage Limits

To keep this service free for everyone, I have to run it on a small, low-cost
server and pay for the LLM API calls myself. To prevent abuse and keep the
costs manageable, I've set some limits on the public proxy.

Currently, the limits are around:

* **10 requests** per minute
* **100 requests** per day (per user/IP)

These limits should be more than enough for a long, productive conversation.
