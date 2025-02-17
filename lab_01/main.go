package main

import (
	"fmt"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"gonum.org/v1/gonum/stat/distuv"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
)

type Request struct {
	arrivalTime float64
	serviceTime float64
	typeID      int
	inQueue     time.Time
}

type Server struct {
	mu       sync.Mutex
	busy     bool
	busyTime int64
}

func (s *Server) Process(req *Request, processed *int, waitTimeSumMilli *int64) {
	s.mu.Lock()
	s.busy = true
	busyTime := time.Duration(req.serviceTime*100) * time.Millisecond
	s.mu.Unlock()

	*waitTimeSumMilli += time.Since(req.inQueue).Milliseconds()
	time.Sleep(busyTime)

	s.mu.Lock()
	s.busy = false
	s.busyTime += busyTime.Milliseconds()
	*processed++
	s.mu.Unlock()
}

func generateRequests(rate float64, serviceRate float64, queue chan Request, id int, stopChan chan struct{}) {
	uniformDist := distuv.Uniform{Min: 1.0 / rate, Max: 2.0 / rate}
	for {
		select {
		case <-stopChan:
			return
		default:
			arrival := uniformDist.Rand()
			service := distuv.Exponential{Rate: serviceRate}.Rand()
			time.Sleep(time.Duration(arrival*100) * time.Millisecond)
			queue <- Request{arrivalTime: arrival, serviceTime: service, typeID: id, inQueue: time.Now()}
		}
	}
}

func generatePlot(dataX, dataY []float64, title, xlabel, ylabel string) *plot.Plot {
	p := plot.New()
	p.Title.Text = title
	p.X.Label.Text = xlabel
	p.Y.Label.Text = ylabel

	pts := make(plotter.XYs, len(dataX))
	for i := range dataX {
		pts[i].X = dataX[i]
		pts[i].Y = dataY[i]
	}

	line, err := plotter.NewLine(pts)
	if err != nil {
		fmt.Println("failed to create line plot:", err)
		return nil
	}

	p.Add(line)
	return p
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("СМО")
	myWindow.Resize(fyne.Size{
		Width:  600,
		Height: 600,
	})

	rateEntry1 := widget.NewEntry()
	rateEntry1.SetPlaceHolder("Интенсивность поступления заявок 1 генератора")

	serviceRateEntry1 := widget.NewEntry()
	serviceRateEntry1.SetPlaceHolder("Интенсивность обслуживания 1 генератора")

	rateEntry2 := widget.NewEntry()
	rateEntry2.SetPlaceHolder("Интенсивность поступления заявок 2 генератора")

	serviceRateEntry2 := widget.NewEntry()
	serviceRateEntry2.SetPlaceHolder("Интенсивность обслуживания 2 генератора")

	requestsCountEntry := widget.NewEntry()
	requestsCountEntry.SetPlaceHolder("Кол-во обработанных заявок для завершения моделирования")

	statusLabel := widget.NewLabel("Ожидание запуска моделирования...")
	loadLabel := widget.NewLabel("Расчетная загрузка: N/A")
	factLoadLabel := widget.NewLabel("Фактическая загрузка: N/A")
	avgWaitLabel := widget.NewLabel("Среднее время ожидания: N/A")

	startButton := widget.NewButton("Старт", func() {
		var rate1, rate2, serviceRate1, serviceRate2 float64
		_, err := fmt.Sscanf(rateEntry1.Text, "%f", &rate1)
		if err != nil {
			statusLabel.SetText("Ошибка: введите корректное значение интенсивности поступления заявок 1 генератора.")
			return
		}
		_, err = fmt.Sscanf(serviceRateEntry1.Text, "%f", &serviceRate1)
		if err != nil {
			statusLabel.SetText("Ошибка: введите корректное значение интенсивности обслуживания заявок 1 генератора.")
			return
		}
		_, err = fmt.Sscanf(rateEntry2.Text, "%f", &rate2)
		if err != nil {
			statusLabel.SetText("Ошибка: введите корректное значение интенсивности поступления заявок 2 генератора.")
			return
		}
		_, err = fmt.Sscanf(serviceRateEntry2.Text, "%f", &serviceRate2)
		if err != nil {
			statusLabel.SetText("Ошибка: введите корректное значение интенсивности обслуживания заявок 2 генератора.")
			return
		}
		var requestsCount int
		_, err = fmt.Sscanf(requestsCountEntry.Text, "%d", &requestsCount)
		if err != nil {
			statusLabel.SetText("Ошибка: введите корректное кол-во заявок для обработки.")
			return
		}

		statusLabel.SetText("Моделирование запущено")

		queue1 := make(chan Request, 10000)
		queue2 := make(chan Request, 10000)
		server := Server{}
		stopChan := make(chan struct{})
		processed := 0
		startTime := time.Now()

		load := rate1/serviceRate1 + rate2/serviceRate2
		loadLabel.SetText(fmt.Sprintf("Расчетная загрузка: %.2f", load))

		go generateRequests(rate1, serviceRate1, queue1, 1, stopChan)
		go generateRequests(rate2, serviceRate2, queue2, 2, stopChan)

		var waitTimeSum1Milli, waitTimeSum2Milli int64
		go func() {
			for {
				select {
				case <-stopChan:
					return
				case req := <-queue1:
					for server.busy {
					}
					go server.Process(&req, &processed, &waitTimeSum1Milli)
				case req := <-queue2:
					for server.busy {
					}
					go server.Process(&req, &processed, &waitTimeSum2Milli)
				}
			}
		}()

		for {
			if processed >= requestsCount {
				stopChan <- struct{}{}

				elapsed := time.Since(startTime).Milliseconds()
				factLoad := float64(server.busyTime) / float64(elapsed)
				factLoadLabel.SetText(fmt.Sprintf("Фактическая загрузка: %.2f", factLoad))

				avgWait1 := float64(waitTimeSum1Milli) / float64(processed)
				avgWait2 := float64(waitTimeSum2Milli) / float64(processed)
				avgWaitLabel.SetText(fmt.Sprintf("Среднее время ожидания 1-й очереди: %.2fмс, 2-й очереди: %.2fмс", avgWait1, avgWait2))

				statusLabel.SetText("Моделирование завершено. " + fmt.Sprintf("Обработано %d заявок", requestsCount))
				break
			}
		}
	})

	//startButton.OnTapped()
	//dataX := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	//dataY := []float64{0.5, 1.0, 1.5, 2.0, 2.5}
	//p := generatePlot(dataX, dataY, "Зависимость времени ожидания от загрузки системы", "Загрузка", "Время ожидания")
	//if p != nil {
	//	err = p.Save(5*vg.Inch, 5*vg.Inch, "waiting_time_vs_load.svg")
	//	if err != nil {
	//		log.Fatal(err)
	//	}
	//}

	myWindow.SetContent(container.NewVBox(
		rateEntry1,
		serviceRateEntry1,
		rateEntry2,
		serviceRateEntry2,
		requestsCountEntry,
		startButton,
		statusLabel,
		loadLabel,
		factLoadLabel,
		avgWaitLabel,
	))

	myWindow.ShowAndRun()
}
