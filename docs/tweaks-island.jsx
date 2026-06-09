/* tweaks-island.jsx — small React island that drives the vanilla page.
   It applies tweak values to document-level state via window.applyTweaks. */
const TWEAK_DEFAULTS = /*EDITMODE-BEGIN*/{
  "hero": "split",
  "accent": "#3fbf8f"
}/*EDITMODE-END*/;

function TweaksIsland() {
  const [t, setTweak] = useTweaks(TWEAK_DEFAULTS);

  // apply on mount + whenever a tweak changes (even when the panel is hidden)
  React.useEffect(() => {
    if (window.applyTweaks) window.applyTweaks(t);
  }, [t.hero, t.accent]);

  return (
    <TweaksPanel title="Tweaks">
      <TweakSection label="Hero treatment" />
      <TweakRadio
        label="Layout"
        value={t.hero}
        options={["split", "receipt", "agentflow"]}
        onChange={(v) => setTweak("hero", v)}
      />
      <TweakSection label="Accent" />
      <TweakColor
        label="Color"
        value={t.accent}
        options={["#3fbf8f", "#d6a23c", "#4f86f7", "#b07be0"]}
        onChange={(v) => setTweak("accent", v)}
      />
    </TweaksPanel>
  );
}

(function mountIsland() {
  const el = document.getElementById("tweaks-root");
  if (el && window.ReactDOM) {
    ReactDOM.createRoot(el).render(<TweaksIsland />);
  }
})();
