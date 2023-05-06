var socket = io("ws://localhost:8080", {
	transports: ['websocket'],
});
var graphs = {}

socket.on("metrics", data => {
	console.log('metrics', JSON.stringify(data));
	if (typeof (data) === 'string') {
		let val = data = data.split(':');
		addSeriesPoint(val[0], val[1]);
	} else if (data !== null && typeof (data) === 'object') {
		$.each(data, (key, val) => {
			val = val.split(':');
			addSeriesPoint(val[0], val[1]);
		});
	}
});

function addSeriesPoint(id, value) {
	id = id.toString();
	if (graphs[id]) {
		var x = (new Date()).getTime(); // current time (TODO?)
		debugger;
		graphs[id].series[0].addPoint([x, 1 * value], true, false)
	}
}

socket.on("symbol_list", data => {
	console.log('Symbols', data);
	if (data !== null) {
		$.each(graphs, graph => {
			graph.destroy();
		});
		$('#container>.chart').remove();
		graphs = {};

		$.each(data, (key, val) => {
			console.log(val);
			graphs[val.ID.toString()] = createGraph(val);
		});

		socket.emit('start_session');
	} else {
		console.log('No symbols!');
	}
});

socket.on('connect', () => {
	console.log('Socket connected!');
	$('#loading-spinner').remove();

	socket.emit('request_symbols');
});
