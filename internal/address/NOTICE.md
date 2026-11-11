# Address metadata attribution

Google libaddressinput address metadata, © Google and contributors.

Source: https://github.com/google/libaddressinput and the official country-level
service https://chromium-i18n.appspot.com/ssl-address (now served from gstatic).
The upstream project identifies the address **data** license as
[Creative Commons Attribution 4.0 International](https://creativecommons.org/licenses/by/4.0/).
Its source code has a separate Apache-2.0 license; Kartwo does not embed the Java
or C++ library.

Kartwo changes: retain only country-level field requirements, labels, examples
and postal syntax; normalize doubled regex escapes; omit names, subdivisions,
external lookup URLs and translations. `rules.json` records the retrieval time,
source and SHA256 of each original response. `tools/sync-address-rules.py` is an
explicit maintainer update tool, never invoked by builds or the running shop.

The offline snapshot does not verify an address exists, matches a postal code,
or is deliverable, and does not enable any destination or payment market.
No customer address is sent to the metadata source.
