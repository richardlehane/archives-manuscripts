package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
)

const layout = "January 2006"

func year(i int) string {
	return fmt.Sprintf("%d", 2016-(44-i))
}

func month(i int) int {
	switch i {
	case 1:
		return 3
	case 2:
		return 7
	case 3:
		return 11
	}
	return 1
}

type volume struct {
	no     int
	issues []int
}

func (v volume) length() int {
	var sum int
	for _, n := range v.issues {
		sum += n
	}
	return sum
}

func (v volume) String() string {
	var one, two, three int
	switch len(v.issues) {
	case 3:
		one, two, three = v.issues[0], v.issues[1], v.issues[2]
	case 2:
		one, two = v.issues[0], v.issues[1]
	case 1:
		one = v.issues[0]
	}
	return fmt.Sprintf("['%s', %d, %d, %d]", year(v.no), one, two, three)
}

var replacer = strings.NewReplacer("é", "e", "ä", "a", "ö", "o", "ó", "o", "“", "'", "”", "'", "‘", "'", "’", "'", "…", "...", "–", "-")

func toAlpha(s string) string {
	s = replacer.Replace(s)
	ret := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsLetter(r) {
			ret = append(ret, unicode.ToLower(r))
		}
		if unicode.IsSpace(r) {
			ret = append(ret, ' ')
		}
	}
	return string(ret)
}

func authorDates(is []Issue) string {
	var l int
	for _, i := range is {
		for _, a := range i.Articles {
			l += len(a.Authors)
		}
	}
	ret := make([]string, 0, l)
	for _, i := range is {
		yr, month := year(i.Volume), month(i.Issue)
		for _, a := range i.Articles {
			for _, auth := range a.Authors {
				ret = append(ret, fmt.Sprintf(atemplate, replacer.Replace(auth), replacer.Replace(a.Title), yr, month-2, yr, month+1))
			}
		}
	}
	return strings.Join(ret, ", ")
}

var atemplate = `[ "%s", null, "%s", new Date(%s, %d, 1), new Date(%s, %d, 1) ]`

type Issue struct {
	Volume   int
	Issue    int
	Date     time.Time
	Articles []Article
}

func (i Issue) length() int {
	var sum int
	for _, v := range i.Articles {
		sum += v.Length
	}
	return sum
}

func (i Issue) titles() string {
	ts := make([]string, len(i.Articles))
	for i, v := range i.Articles {
		ts[i] = toAlpha(v.Title)
	}
	return strings.Join(ts, " ")
}

type Article struct {
	Authors []string
	Title   string
	Start   int
	End     int
	Length  int
}

func (a Article) length() int {
	return a.End - a.Start + 1
}

func pagenums(s string) (int, int) {
	var start, end int
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "pp. ")
	s = strings.TrimPrefix(s, "p. ")
	i, err := fmt.Sscanf(s, "%d-%d", &start, &end)
	switch i {
	case 1:
		return start, start
	case 2:
		return start, end
	}
	fmt.Println(err)
	os.Exit(1)
	return -1, -1
}

func main() {
	// PROCESS LEGACY ENTRIES
	f, err := os.Open("legacy.txt")
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	scanner := bufio.NewScanner(f)
	var issues []Issue
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "Articles"),
			strings.HasPrefix(line, "Reflection"),
			strings.HasPrefix(line, "Guest Editorial"),
			strings.HasPrefix(line, "Case Study"),
			strings.HasPrefix(line, "Review Article"),
			strings.HasPrefix(line, "Interview"):
			continue
		}
		if strings.HasPrefix(line, "Volume") {
			var issue Issue
			var vt, nt, mt, yt string
			i, err := fmt.Sscanf(line, "%s%d%s%d%s%s", &vt, &issue.Volume, &nt, &issue.Issue, &mt, &yt)
			if i != 6 || err != nil {
				fmt.Printf("Bad scan: %s; err: %v; num: %d; vals: %s, %d, %s, %d, %s, %s\n", line, err, i, vt, issue.Volume, nt, issue.Issue, mt, yt)
				os.Exit(1)
			}
			issue.Date, err = time.Parse(layout, mt+" "+yt)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			issues = append(issues, issue)
			continue
		}
		var art Article
		els := strings.Split(line, ",")
		if len(els) < 3 {
			fmt.Printf("Bad length:%s\n", line)
			os.Exit(1)
		}
		var pages string
		if len(els) > 3 {
			pages = els[len(els)-1]
			var titleStart int
			for i, v := range els {
				if strings.Index(v, "‘") > -1 {
					titleStart = i
					break
				}
				auths := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "and "), " and ")
				art.Authors = append(art.Authors, auths...)
			}
			art.Title = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(strings.Join(els[titleStart:len(els)-1], ",")), "‘"), "’")
		} else {
			art.Authors, art.Title, pages = strings.Split(els[0], " and "), strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(els[1]), "‘"), "’"), els[2]
		}
		art.Start, art.End = pagenums(pages)
		art.Length = art.length()
		issues[len(issues)-1].Articles = append(issues[len(issues)-1].Articles, art)
		for i, v := range art.Authors {
			if idx := strings.Index(v, " ("); idx > -1 {
				v = v[:idx]
			}
			art.Authors[i] = strings.TrimSpace(v)
		}
	}
	f.Close()
	// PROCESS RECENT ENTRIES
	f, err = os.Open("ongoing.bib")
	defer f.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	scanner = bufio.NewScanner(f)
	scanner.Split(split)
	var lineScanner *bufio.Scanner
	for scanner.Scan() {
		lineScanner = bufio.NewScanner(bytes.NewReader(scanner.Bytes()))
		var art Article
		var volN, issN int
		var year string
		for lineScanner.Scan() {
			line := lineScanner.Text()
			switch {
			case strings.HasPrefix(line, "author"):
				art.Authors = authors(value(line))
			case strings.HasPrefix(line, "title"):
				art.Title = value(line)
			case strings.HasPrefix(line, "volume"):
				_, err = fmt.Sscanf(value(line), "%d", &volN)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			case strings.HasPrefix(line, "number"):
				_, err = fmt.Sscanf(value(line), "%d", &issN)
				if err != nil {
					fmt.Println(err)
					os.Exit(1)
				}
			case strings.HasPrefix(line, "year"):
				year = value(line)
			case strings.HasPrefix(line, "pages"):
				art.Start, art.End = pagenums(value(line))
				art.Length = art.length()
			}
		}
		if issues[len(issues)-1].Volume != volN || issues[len(issues)-1].Issue != issN {
			switch issN {
			case 1:
				year = "March " + year
			case 2:
				year = "July " + year
			case 3:
				year = "November " + year
			default:
				year = "December " + year
			}
			dt, err := time.Parse(layout, year)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
			issues = append(issues, Issue{Volume: volN, Issue: issN, Date: dt})
		}
		issues[len(issues)-1].Articles = append(issues[len(issues)-1].Articles, art)
	}
	// Populate volumes
	var volumes []volume
	var tstrs []string
	var last int
	for _, issue := range issues {
		if last != issue.Volume {
			volumes = append(volumes, volume{no: issue.Volume, issues: []int{issue.length()}})
			if last == 0 || issue.Volume == 39 {
				tstrs = append(tstrs, issue.titles())
			} else {
				tstrs[len(tstrs)-1] = tstrs[len(tstrs)-1] + " " + issue.titles()
			}
			last = issue.Volume
		} else {
			volumes[len(volumes)-1].issues = append(volumes[len(volumes)-1].issues, issue.length())
			tstrs[len(tstrs)-1] = tstrs[len(tstrs)-1] + " " + issue.titles()
		}
	}
	volstrs := make([]string, len(volumes))
	for i, v := range volumes {
		volstrs[i] = v.String()
	}
	var jtemps, htemps = make([]string, 0, len(tstrs)), make([]string, 0, len(tstrs))
	for i, v := range tstrs {
		yr := "2005-2010"
		if i > 0 {
			yr = "2011-2016"
		}
		jt, ht := histtemplates(yr, wordCount(v).String())
		jtemps, htemps = append(jtemps, jt), append(htemps, ht)
	}
	of, err := os.Create("index.html")
	defer of.Close()
	fmt.Fprintf(of, template, strings.Join(volstrs, ", "), authorDates(issues), strings.Join(htemps, "\n"), strings.Join(jtemps, "\n"))
	// WRITE OUTPUT
	b, err := json.Marshal(issues)
	if err != nil {
		fmt.Println("error:", err)
	}
	ioutil.WriteFile("am.json", b, 0777)
}

var startTok = []byte("@article{")

func split(data []byte, atEOF bool) (advance int, token []byte, err error) {
	idx := bytes.Index(data, startTok)
	if idx < 0 {
		if atEOF {
			return 0, nil, io.EOF
		}
		return 0, nil, nil
	}
	var open int
	for i, v := range data[idx+len(startTok):] {
		if v == '{' {
			open++
		}
		if v == '}' {
			if open > 0 {
				open--
			} else {
				return idx + len(startTok) + i + 1, data[idx : idx+len(startTok)+i+1], nil
			}
		}
	}
	return 0, nil, nil
}

func value(s string) string {
	start, end := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if start < 0 || end < 0 || end < start {
		return ""
	}
	return s[start+1 : end]
}

func authors(s string) []string {
	authors := strings.Split(s, ",")
	last := strings.Split(authors[len(authors)-1], "and")
	authors = append(authors[:len(authors)-1], last...)
	for i, v := range authors {
		if idx := strings.Index(v, "("); idx > -1 {
			v = v[:idx]
		}
		authors[i] = strings.TrimSpace(v)

	}
	return authors
}

var template = `
<html>
  <head>
    <script type="text/javascript" src="https://www.gstatic.com/charts/loader.js"></script>
    <script type="text/javascript" src="wordcloud2.js"></script>
    <script type="text/javascript">
      google.charts.load('current', {'packages':['bar', 'timeline', 'corechart']});
      google.charts.setOnLoadCallback(drawCharts);
      function drawCharts() {
        var data = google.visualization.arrayToDataTable([
          ['Year', 'Issue 1', 'Issue 2', 'Issue 3'],
          %s
        ]);
        var options = {
        bars: 'horizontal',
        isStacked: true
      	};
        var barchart = new google.charts.Bar(document.getElementById('barchart_material'));
        barchart.draw(data, google.charts.Bar.convertOptions(options));

        var dataTable = new google.visualization.DataTable();
        dataTable.addColumn({ type: 'string', id: 'Author' });
        dataTable.addColumn({ type: 'string', id: 'dummy bar label' });
	        dataTable.addColumn({ type: 'string', role: 'tooltip' });
        dataTable.addColumn({ type: 'date', id: 'Start' });
        dataTable.addColumn({ type: 'date', id: 'End' });
        dataTable.addRows([
          %s]);

        var timelinechart = new google.visualization.Timeline(document.getElementById('timeline'));
        timelinechart.draw(dataTable);
      }
     </script>
  </head>
  <body>
  	<h1>A&M stats</h1>
  	<p>Raw data for these stats available here: <a href="am.json">am.json</a>. Source data from <a href="http://www.archivists.org.au/learning-publications/archives-and-manuscripts/back-issues-of-the-journal">the ASA website</a>.</p>
  	<h2>Authors</h2>
	<div id="timeline" style="height: 750px;"></div>
  	<h2>Titles</h2>
	%s
    <h2>Lengths</h2>
	<div id="barchart_material" style="width: 900px; height: 500px;"></div>
  </body>
  <script>%s</script>
</html>
`

func histtemplates(yr string, data string) (string, string) {
	jstemp := `WordCloud(document.getElementById('%s_canvas'), { list: %s,
	weightFactor: function (size) {
    return size + 4;
  }	
  	})`
	htemp := `<div><h2>%s</h2><canvas style="width: 400; height: 400;" id="%s_canvas"></canvas></div>`
	return fmt.Sprintf(jstemp, yr, data), fmt.Sprintf(htemp, yr, yr)
}

var stopWords = []string{"a", "able", "about", "across", "after", "all", "almost", "also", "am", "among", "an", "and", "any", "are", "as", "at", "be", "because", "been", "but", "by", "can", "cannot", "could", "dear", "did", "do", "does", "either", "else", "ever", "every", "for", "from", "get", "got", "had", "has", "have", "he", "her", "hers", "him", "his", "how", "however", "i", "if", "in", "into", "is", "it", "its", "just", "least", "let", "like", "likely", "may", "me", "might", "most", "must", "my", "neither", "no", "nor", "not", "of", "off", "often", "on", "only", "or", "other", "our", "own", "rather", "said", "say", "says", "she", "should", "since", "so", "some", "than", "that", "the", "their", "them", "then", "there", "these", "they", "this", "tis", "to", "too", "twas", "us", "wants", "was", "we", "were", "what", "when", "where", "which", "while", "who", "whom", "why", "will", "with", "would", "yet", "you", "your", "ain't", "aren't", "can't", "could've", "couldn't", "didn't", "doesn't", "don't", "hasn't", "he'd", "he'll", "he's", "how'd", "how'll", "how's", "i'd", "i'll", "i'm", "i've", "isn't", "it's", "might've", "mightn't", "must've", "mustn't", "shan't", "she'd", "she'll", "she's", "should've", "shouldn't", "that'll", "that's", "there's", "they'd", "they'll", "they're", "they've", "wasn't", "we'd", "we'll", "we're", "weren't", "what'd", "what's", "when'd", "when'll", "when's", "where'd", "where'll", "where's", "who'd", "who'll", "who's", "why'd", "why'll", "why's", "won't", "would've", "wouldn't", "you'd", "you'll", "you're", "you've"}

func stopword(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

type word struct {
	val string
	num int
}

func (w word) String() string {
	return fmt.Sprintf("['%s', %d]", w.val, w.num)
}

type words []word

func (w words) Len() int           { return len(w) }
func (w words) Swap(i, j int)      { w[i], w[j] = w[j], w[i] }
func (w words) Less(i, j int) bool { return w[i].num > w[j].num }

func (w words) String() string {
	if len(w) == 0 {
		return ""
	}
	strs := make([]string, len(w))
	for i, v := range w {
		strs[i] = v.String()
	}
	return "[" + strings.Join(strs, ", ") + "]"
}

func wordCount(s string) words {
	m := make(map[string]int)
	for _, w := range strings.Fields(s) {
		if stopword(w, stopWords) {
			continue
		}
		m[w] += 1
	}
	ws := make(words, 0, len(m))
	for k, v := range m {
		if v > 1 {
			ws = append(ws, word{k, v})
		}
	}
	sort.Sort(ws)
	return ws
}
