package flywheel

/*
This could probably be smarter, but it'll do for now.
*/

// HTMLSTOPPED - display when system is stopped
const HTMLSTOPPED = `
	<html>
		<body style="color: #333333; background: #f5f5f5;">
			<h1 style="text-align: center; margin-top: 50px; font-size: larger;">Your service is currently powered down</h1>
			<p style="text-align: center;"><a href="%s">Click here</a> to start.</p>
		</body>
	</html>`

// HTMLSTARTING - display when system is starting
const HTMLSTARTING = `
	<html>
		<script>
			setTimeout(function() {
				window.location.reload(1);
			}, 5000);
		</script>
		<body style="color: #333333; background: #f5f5f5">
			<h1 style="text-align: center; margin-top: 50px; font-size: larger;">Your service is starting, please wait.</h1>
			<p style="text-align: center;">Your site will be loaded once startup is complete.</p>
		</body>
	</html>`

// HTMLSTOPPING - display when system is stopping
const HTMLSTOPPING = `
	<html>
		<script>
			setTimeout(function() {
				window.location.reload(1);
			}, 5000);
		</script>
		<body style="color: #333333; background: #f5f5f5">
			<h1 style="text-align: center; margin-top: 50px; font-size: larger;">Your service is being powered down.</h1>
			<p style="text-align: center;">Please wait for shutdown to complete before restarting.</p>
		</body>
	</html>`

// HTMLUNHEALTHY - display when system is unhealthy
const HTMLUNHEALTHY = `
	<html>
		<body style="color: #333333; background: #f5f5f5">
			<h1 style="text-align: center; margin-top: 50px; font-size: larger;">Your service appears to be in an unhealthy or inconsistent state</h1>
			<p style="text-align: center;">This may be a temporary error, or may require manual intervention.</p>
		</body>
	</html>`

// HTMLERROR - display when error
const HTMLERROR = `
	<html>
		<body style="color: #333333; background: #f5f5f5">
			<h1 style="text-align: center; margin-top: 50px; font-size: larger;">An error occured processing your request</h1>
			<p style="text-align: center;">%v</p>
		</body>
	</html>`
