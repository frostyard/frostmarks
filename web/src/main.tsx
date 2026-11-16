// The web UI's entry: it runs only inside lodge's shell. Outside a frame,
// lodge-client replaces the page with the app's canonical URL, so the shell
// loads it (lodge docs/specs/lodge-client.md).
import "./styles/app.css";

import { connectLodge } from "@frostyard/lodge-client";
import { followLodge } from "@frostyard/lodge-ui/theme";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./App";
import { connectAPI } from "./api";

const root = document.getElementById("root");
if (!root) throw new Error("missing #root");
const lodge = await connectLodge({ app: "marks" });
followLodge(lodge);
createRoot(root).render(
  <StrictMode>
    <App lodge={lodge} api={connectAPI()} />
  </StrictMode>,
);
