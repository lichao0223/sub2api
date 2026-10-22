# DeepSeek Input Tokenizer

The gateway uses this tokenizer as its common text-counting standard for input
admission, regardless of the requested model. This is not provider billing usage:
other models and their chat templates can produce different counts. The request
walker still excludes metadata and binary payloads and estimates image tokens.

## Pinned Asset

- Source: https://huggingface.co/deepseek-ai/DeepSeek-V4-Flash
- Revision: `60d8d70770c6776ff598c94bb586a859a38244f1`
- File: `tokenizer.json`, embedded unchanged; no runtime download is required.
- SHA-256: `8f9f37ca37fdc4f5fd36d5cf4d3b0e8392edb4e894fd10cc0d70b4957c8633cf`
- License: MIT, preserved in `LICENSE`.

The Go adapter applies the asset's added tokens, three isolated regex splits,
ByteLevel byte mapping, and ranked BPE merges (leftmost first for equal ranks).
This pinned asset has no normalization, prefix space, or inserted BOS/EOS token.
Do not replace it with a tokenizer requiring other preprocessing without updating
the adapter and reference fixtures together.

## Reference Tests

`golden.json` contains 103 synthetic examples, generated with the official
Hugging Face runtime `tokenizers==0.22.2`:

```python
from tokenizers import Tokenizer

tokenizer = Tokenizer.from_file("tokenizer.json")
tokens = len(tokenizer.encode(text, add_special_tokens=False).ids)
```

The fixtures cover mixed languages, code, numeric splitting, whitespace, emoji,
added tokens, long BPE pieces, and deterministic random text. All added tokens
are also checked separately. No user request content is included.
