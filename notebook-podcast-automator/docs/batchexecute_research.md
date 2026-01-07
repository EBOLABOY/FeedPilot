# Google BatchExecute Protocol & Cookie Reliability Research

## 1. BatchExecute Protocol Analysis

The `batchexecute` endpoint is a generic RPC (Remote Procedure Call) mechanism used by many Google web applications (Search, Maps, Play, Gemini/Bard, NotebookLM) to bundle multiple API requests into a single HTTP POST.

### "Encrypted" Data Fallacy
Users often confusingly refer to the data payload as "encrypted".
- **Reality**: The payload is **NOT encrypted** with a secret key (like AES).
- **Structure**: It is a nested, serialized format.
    - **Wrappers**: The outer layer is often a JSON array or a specific delimiter format like `)]}'\n`.
    - **Inner Payload (`f.req`)**: This parameter contents are usually a nested JSON array: `[[["rpc_id", "json_payload", null, "generic"]]]`.
    - **Deep Payload**: The `json_payload` inside is often *another* JSON string, which itself might represent a Protobuf message where field names are stripped and replaced by array indices (e.g., `["value", null, 1]`).
- **Why it looks random**: The intense nesting and lack of semantic keys (use of indices) make it opaque to human readers, but it is standard serialization, not encryption.

### Open Source References
Developers have reverse-engineered this for various tools:
*   [dsdanielpark/Bard-API](https://github.com/dsdanielpark/Bard-API): Python wrapper for Google Bard that heavily uses `batchexecute`.
*   [acheong08/Bard](https://github.com/acheong08/Bard): Another Python implementation.
*   [yt-dlp](https://github.com/yt-dlp/yt-dlp): Reverse engineers similar generic endpoints for YouTube data extraction.

## 2. Deep Dive: RPC IDs and `f.req`

### What is `f.req`?
`f.req` is the standard form field name Google uses to submit the batch payload. In many contexts, `req` just stands for "request". The content is a stringified JSON array of arrays.

### RPC IDs (`rpcids`)
- **Nature**: These are short alphanumeric strings (e.g., `vyAe2`, `qnKhOb`) that map to specific server-side functions.
- **Dynamic**: They are **not** global or static. `vyAe2` on Google Play might fetch charts, while the same ID might not exist or do something different on Google Maps.
- **Discovery**: There is no master list. They must be discovered by inspecting network traffic (Developer Tools -> Network -> Filter "batchexecute") for the specific application you are targeting.
- **Obfuscation**: Google compiles their backend (likely Java/C++) and frontend (Closure Compiler), which minifies function names into these short IDs.

## 3. Cookie Expiration & Reliability

Rapid cookie expiration (session invalidation) in automated scripts is typically caused by Google's anti-abuse systems detecting non-browser behavior.

### High-Risk Triggers (Avoid These)
1.  **TLS Fingerprinting**: Go's default `net/http` has a unique JA3 fingerprint.
    *   *Fix*: Use `uTLS` (Golang) or `curl-impersonate` (other languages) to spoof Chrome's TLS handshake.
2.  **Missing Client Hints**: Modern browsers send `Sec-Ch-Ua` headers. Absence is suspicious.
3.  **IP Mismatch**: Auth cookies created on a residential IP but used on a cloud server IP often die instantly.
4.  **Header "Lie"**: Sending `User-Agent: Chrome` without accompanying headers like `Sec-Ch-Ua-Platform="Windows"` decreases trust score.

### Recommended Headers (Implemented)
To mimic a modern browser (e.g., Chrome on Windows) and improve session longevity, ensure these headers are present and consistent:

```http
User-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36
Sec-Ch-Ua: "Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"
Sec-Ch-Ua-Mobile: ?0
Sec-Ch-Ua-Platform: "Windows"
Sec-Ch-Ua-Arch: "x86"
Sec-Ch-Ua-Full-Version: "120.0.6099.130"
Accept: */*
Accept-Language: en-US,en;q=0.9
X-Goog-AuthUser: 0
```
