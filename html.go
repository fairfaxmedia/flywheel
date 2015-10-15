package main

/*
This could probably be smarter, but it'll do for now.
*/

const HTML_STOPPED = `
	<html>
		<body>
			<p>The instances for this service are currently powered down.</p>
			<p><a href="%s">Click here</a> to start.</p>
		</body>
	</html>`

const HTML_STARTING = `
	<html>
		<body>
			<p>Your service is starting, please wait.</p>
		</body>
	</html>`

const HTML_STOPPING = `
	<html>
		<body>
			<p>The instances for this service are being powered down.</p>
		</body>
	</html>`

const HTML_UNHEALTHY = `
	<html>
		<body>
			<p>The instances for this service appear to be in an unhealthy or inconsistent state.</p>
		</body>
	</html>`

const HTML_ERROR = `
	<html>
		<body>
			<p>An error occured processing your request: %v</p>
		</body>
	</html>`
