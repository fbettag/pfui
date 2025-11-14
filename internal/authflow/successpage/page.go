package successpage

const html = `<!DOCTYPE html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>pfui Login</title>
    <style>
      body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; padding: 0; margin: 0; background: #f5f5f5; }
      main { max-width: 460px; margin: 12vh auto; background: #fff; border-radius: 16px; padding: 32px; box-shadow: 0 10px 30px rgba(0,0,0,.07); text-align: center; }
      h1 { font-size: 1.25rem; margin-bottom: .5rem; }
      p { color: #444; margin-bottom: 1rem; }
      small { color: #777; }
    </style>
  </head>
  <body>
    <main>
      <h1>Signed in to pfui</h1>
      <p>You can close this tab. We’ll do it automatically in a moment.</p>
      <small>If it doesn’t close, it’s safe to manually close the window.</small>
    </main>
    <script>
      setTimeout(function() {
        window.close();
      }, 1200);
    </script>
  </body>
</html>`

// HTML returns the success page snippet shared by OAuth flows.
func HTML() string {
	return html
}
