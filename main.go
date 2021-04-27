package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/360EntSecGroup-Skylar/excelize/v2"
)

func main() {

	createServer()
}

// 创建一个tcp的socket
func createServer() {
	//建立socket，监听端口  第一步:绑定端口
	//netListen, err := net.Listen("tcp", "localhost:1024")
	netListen, err := net.Listen("tcp", "127.0.0.1:9889")
	CheckError(err)
	//defer延迟关闭改资源，以免引起内存泄漏
	defer netListen.Close()

	Log("Waiting for clients")
	for {
		conn, err := netListen.Accept() //第二步:获取连接
		if err != nil {
			continue //出错退出当前一次循环
		}

		Log(conn.RemoteAddr().String(), " tcp connect success")
		//handleConnection(conn)  //正常连接就处理
		//这句代码的前面加上一个 go，就可以让服务器并发处理不同的Client发来的请求
		go handleConnection(conn) //使用goroutine来处理用户的请求
	}
}

// 处理连接
func handleConnection(conn net.Conn) {

	buffer := make([]byte, 2048)

	for { //无限循环

		n, err := conn.Read(buffer) //第三步:读取从该端口传来的内容
		// words := "ok"               //向链接中写数据,向链接既可以先读也可以先写，看自己的需要
		// words := "golang socket server : " + strconv.Itoa(rand.Intn(100)) //向链接中写数据
		// conn.Write([]byte(words))
		if err != nil {
			Log(conn.RemoteAddr().String(), " connection error: ", err)
			return //出错后返回
		}

		tcpData := string(buffer[:n])

		if string(tcpData[0]) == "[" {
			// 第一次通信会传一个路径数组过来，如果用go来生成文件则数组默认最后一个传生成文件所在文件夹路径
			var sourcePathList []string
			json.Unmarshal([]byte(tcpData), &sourcePathList)

			// go处理生成文件
			fmt.Println("开始处理...")
			startTime := time.Now().Unix()
			folderPath := sourcePathList[len(sourcePathList)-1]
			mode := sourcePathList[len(sourcePathList)-2]
			sourcePathList = sourcePathList[:len(sourcePathList)-2]
			totalFileNum := 0
			for sourceIndex, sourcePath := range sourcePathList {
				time.Sleep(time.Duration(10) * time.Millisecond)
				conn.Write([]byte("正在打开第" + strconv.Itoa(sourceIndex+1) + "个文件"))
				f, _ := excelize.OpenFile(sourcePath)

				sheetList := f.GetSheetMap()
				for _, sheetName := range sheetList {
					time.Sleep(time.Duration(10) * time.Millisecond)
					conn.Write([]byte("正在解析第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "工作表..."))
					rows, _ := f.GetRows(sheetName)
					rowsLen := len(rows)

					// 进行空表校验
					if rowsLen == 0 {
						time.Sleep(time.Duration(10) * time.Millisecond)
						conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表解析出来是空表"))
						return
					}

					if mode == "1" {
						// 进行字段数量校验
						row0Len := len(rows[0])
						if row0Len < 10 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表缺少了" + strconv.Itoa(10-row0Len) + "个列字段"))
							return
						} else if row0Len > 11 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表多出了" + strconv.Itoa(row0Len-11) + "个列字段"))
							return
						}

						// 进行字段名字和顺序校验
						errNameList := []string{}
						keyNameList := []string{"账户号", "交易描述", "卡号", "交易日期", "币种", "交易金额", "余额", "姓名", "证件号码", "编号", "案号"}
						for i, name := range rows[0] {
							if name != keyNameList[i] {
								colName, _ := excelize.ColumnNumberToName(i + 1)
								errNameList = append(errNameList, "第 "+colName+` 列"`+name+`"应为"`+keyNameList[i]+`"`)
							}
						}
						if len(errNameList) > 0 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表列字段" + joinListString(errNameList, "、")))
							return
						}

						// 进行空行和编号校验
						emptyOrderAccouts := []string{}
						for i := rowsLen - 1; i >= 0; i-- {
							rowLen := len(rows[i])
							if rowLen == 0 || (rowLen > 0 && rows[i][0] == "") {
								rows = append(rows[:i], rows[i+1:]...)
							} else if (rows[i][9] == "" || rows[i][9] == "#N/A") && !isValueInList(rows[i][0], emptyOrderAccouts) {
								emptyOrderAccouts = append(emptyOrderAccouts, rows[i][0])
							}
						}
						if len(emptyOrderAccouts) > 0 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表账号" + joinListString(emptyOrderAccouts, "、") + "的编号是空的"))
							return
						}

						rowsHeader := handleRow(rows[0], mode)
						rowsBody := rows[1:]
						breakPoint := 0
						rowsBodyLen := len(rowsBody)

						// 按账户号进行一次排序，保证相同账户号的行挨在一起
						mergeSort(rowsBody, 0, rowsBodyLen-1, 0)

						for i, row := range rowsBody {
							if (i < rowsBodyLen-1 && row[9] != rowsBody[i+1][9]) || i == rowsBodyLen-1 {
								rowsBodySlice := [][]string{{}}
								if i == rowsBodyLen-1 {
									rowsBodySlice = rowsBody[breakPoint:]
								} else {
									rowsBodySlice = rowsBody[breakPoint : i+1]
								}
								breakPoint = i + 1
								caseNum := ""
								if row0Len == 11 && len(rowsBodySlice[0]) == 11 {
									caseNum = rowsBodySlice[0][10]
								}
								order := rowsBodySlice[0][9]
								accout := rowsBodySlice[0][0]
								name := rowsBodySlice[0][7]
								id := rowsBodySlice[0][8]
								templateOutput := [][]string{
									{order, "", "", "", "", ""},
									{"上海浦东发展银行个人信用卡账户对账单", "", "", "", "", ""},
									{"案号：", caseNum, "", "账户号：", accout, ""},
									{"姓名：", name, "", "证件号码：", id, ""},
								}
								for i := range rowsBodySlice {
									rowsBodySlice[i] = handleRow(rowsBodySlice[i], mode)
								}
								templateOutput = append(append(templateOutput, rowsHeader), rowsBodySlice...)

								// fBase, _ := excelize.OpenFile("/Users/huangqier/Downloads/go成品模板.xlsx")
								// sheetNameListBase := fBase.GetSheetMap()
								// var sheetNameBase string
								// for _, name := range sheetNameListBase {
								// 	sheetNameBase = name
								// }

								// 模板列宽和行高
								colWidthList := [6]float64{8.71875 - 0.71, 8.0703125 - 0.71, 9.5 - 0.71, 28.1171875 - 0.71, 13.8046875 - 0.71, 9.375 - 0.71}
								rowHeightList := [2]float64{10.8, 20.4}
								// for i := range colWidthList {
								// 	colName, _ := excelize.ColumnNumberToName(i + 1)
								// 	width, _ := fBase.GetColWidth(sheetNameBase, colName)
								// 	colWidthList[i] = width
								// }
								// for i := range rowHeightList {
								// 	height, _ := fBase.GetRowHeight(sheetNameBase, i+1) // 这里获取行高可能有bug
								// 	rowHeightList[i] = height
								// }

								// 模板打印格式
								var (
									marginBottom excelize.PageMarginBottom = 0.313888888888889
									marginFooter excelize.PageMarginFooter = 0.118055555555556
									marginHeader excelize.PageMarginHeader = 0.118055555555556
									marginLeft   excelize.PageMarginLeft   = 0.751388888888889
									marginRight  excelize.PageMarginRight  = 0.590277777777778
									marginTop    excelize.PageMarginTop    = 0.313888888888889
								)
								// fBase.GetPageMargins(sheetNameBase,
								// 	&marginBottom,
								// 	&marginFooter,
								// 	&marginHeader,
								// 	&marginLeft,
								// 	&marginRight,
								// 	&marginTop,
								// )

								// fmt.Println("colWidthList", colWidthList)
								// fmt.Println("rowHeightList", rowHeightList)
								// fmt.Println("- marginBottom:", marginBottom)
								// fmt.Println("- marginFooter:", marginFooter)
								// fmt.Println("- marginHeader:", marginHeader)
								// fmt.Println("- marginLeft:", marginLeft)
								// fmt.Println("- marginRight:", marginRight)
								// fmt.Println("- marginTop:", marginTop)

								f := excelize.NewFile()
								sheetNameList := f.GetSheetMap()
								var sheetName string
								for _, name := range sheetNameList {
									sheetName = name
								}

								for i, row := range templateOutput {
									for j, cellValue := range row {
										cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
										if i > 4 {
											if j == 2 || j == 5 {
												float, _ := strconv.ParseFloat(cellValue, 64)
												if float < 0.01 && float > -0.01 {
													float = 0
												}
												f.SetCellValue(sheetName, cell, float)
											} else if j == 1 {
												timeValue, _ := time.Parse("01-02-06", cellValue)
												// 月-日-年的时间格式处理成excel的时间戳，计算公式(秒时间戳+8*3600)/86400+70*365+19
												timeFloat := (timeValue.Unix()+8*3600)/86400 + 70*365 + 19
												f.SetCellValue(sheetName, cell, timeFloat)
											} else {
												f.SetCellValue(sheetName, cell, cellValue)
											}
										} else {
											f.SetCellValue(sheetName, cell, cellValue)
										}
									}
								}

								rowLen := len(templateOutput)
								colLen := len(templateOutput[rowLen-1])
								lastCellAxis, _ := excelize.CoordinatesToCellName(colLen, rowLen)
								lastDateCellAxis, _ := excelize.CoordinatesToCellName(2, rowLen)

								// 列宽和行高
								for i, colWidth := range colWidthList {
									colName, _ := excelize.ColumnNumberToName(i + 1)
									f.SetColWidth(sheetName, colName, colName, math.Trunc((colWidth*7+5)/7*256)/256)
								}
								for i := range templateOutput {
									if i == 1 {
										f.SetRowHeight(sheetName, i+1, rowHeightList[1])
									} else {
										f.SetRowHeight(sheetName, i+1, rowHeightList[0])
									}
								}

								// 单元格格式
								styleBaseStr := `"alignment":{"horizontal":"center","vertical":"center"},"font":{"family":"宋体","size":7}`
								styleBorderStr := `"border":[{"type":"left","color":"000000","style":1},{"type":"top","color":"000000","style":1},{"type":"bottom","color":"000000","style":1},{"type":"right","color":"000000","style":1}]`
								styleDateStr := `"number_format":14`
								styleFontSizeStr := `"font":{"family":"宋体","size":14}`
								styleAlignmentRightStr := `"alignment":{"horizontal":"right","vertical":"center"}`
								styleAlignmentLeftStr := `"alignment":{"horizontal":"left","vertical":"center"}`

								styleBase, _ := f.NewStyle("{" + styleBaseStr + "}")
								err := f.SetCellStyle(sheetName, "A1", lastCellAxis, styleBase)

								styleBorder, _ := f.NewStyle("{" + styleBaseStr + "," + styleBorderStr + "}")
								err = f.SetCellStyle(sheetName, "A5", lastCellAxis, styleBorder)

								styleDate, _ := f.NewStyle("{" + styleBaseStr + "," + styleBorderStr + "," + styleDateStr + "}")
								err = f.SetCellStyle(sheetName, "B6", lastDateCellAxis, styleDate)

								styleFontSize, _ := f.NewStyle("{" + styleBaseStr + "," + styleFontSizeStr + "}")
								err = f.SetCellStyle(sheetName, "A2", "A2", styleFontSize)

								styleAlignmentRight, _ := f.NewStyle("{" + styleBaseStr + "," + styleAlignmentRightStr + "}")
								err = f.SetCellStyle(sheetName, "A3", "A3", styleAlignmentRight)
								err = f.SetCellStyle(sheetName, "A4", "A4", styleAlignmentRight)
								err = f.SetCellStyle(sheetName, "D3", "D3", styleAlignmentRight)
								err = f.SetCellStyle(sheetName, "D4", "D4", styleAlignmentRight)

								f.SetCellValue(sheetName, "D3", "账  户  号：")
								f.MergeCell(sheetName, "A1", "F1")
								f.MergeCell(sheetName, "A2", "F2")
								f.MergeCell(sheetName, "B3", "C3")
								f.MergeCell(sheetName, "B4", "C4")

								styleAlignmentLeft, _ := f.NewStyle("{" + styleBaseStr + "," + styleAlignmentLeftStr + "}")
								err = f.SetCellStyle(sheetName, "B3", "B3", styleAlignmentLeft)
								err = f.SetCellStyle(sheetName, "B4", "B4", styleAlignmentLeft)
								err = f.SetCellStyle(sheetName, "E3", "E3", styleAlignmentLeft)
								err = f.SetCellStyle(sheetName, "E4", "E4", styleAlignmentLeft)

								// 打印格式
								f.SetPageMargins(sheetName,
									excelize.PageMarginBottom(marginBottom),
									excelize.PageMarginFooter(marginFooter),
									excelize.PageMarginHeader(marginHeader),
									excelize.PageMarginLeft(marginLeft),
									excelize.PageMarginRight(marginRight),
									excelize.PageMarginTop(marginTop),
								)
								f.SetHeaderFooter(sheetName, &excelize.FormatHeaderFooter{
									DifferentFirst:   false,
									DifferentOddEven: false,
									OddHeader:        "",
									OddFooter:        "第 &P 页，共 &N 页",
									EvenHeader:       "",
									EvenFooter:       "第 &P 页，共 &N 页",
									FirstHeader:      "",
								})

								err = f.Save()

								path := folderPath + "/账户对账单/" + order + name
								fullPath := path + "/" + order + "交易流水" + name + ".xlsx"

								err = os.MkdirAll(path, os.ModePerm)
								if err != nil {
									fmt.Println(err)
									return
								}
								if err := f.SaveAs(fullPath); err != nil {
									fmt.Println(err)
									return
								}
								totalFileNum++
								conn.Write([]byte("成功生成第" + strconv.Itoa(totalFileNum) + "个文件"))
							}
						}
					} else if mode == "2" {
						// 进行字段数量校验
						row0Len := len(rows[0])
						if row0Len < 21 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表缺少了" + strconv.Itoa(21-row0Len) + "个列字段"))
							return
						}

						// 进行字段名字和顺序校验
						errNameList := []string{}
						keyNameList := []string{"证件号", "分期申请编号", "期数", "放款金额", "放款年月", "账号", "放款日期", "交易利息/元", "手续费/元", "分期类型", "渠道", "细分渠道", "卡号", "万用金利率", "分期付款类型", "总息费/元", "地址2", "转入卡号", "最大期数", "客户姓名", "案件编号"}
						for i, name := range rows[0] {
							if name != keyNameList[i] {
								colName, _ := excelize.ColumnNumberToName(i + 1)
								errNameList = append(errNameList, "第 "+colName+` 列"`+name+`"应为"`+keyNameList[i]+`"`)
							}
						}
						if len(errNameList) > 0 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表列字段" + joinListString(errNameList, "、")))
							return
						}

						// 进行空行和案件编号校验
						emptyOrderAccouts := []string{}
						for i := rowsLen - 1; i >= 0; i-- {
							rowLen := len(rows[i])
							if rowLen == 0 || (rowLen > 0 && rows[i][5] == "") {
								rows = append(rows[:i], rows[i+1:]...)
							} else if (rows[i][20] == "" || rows[i][20] == "#N/A") && !isValueInList(rows[i][5], emptyOrderAccouts) {
								emptyOrderAccouts = append(emptyOrderAccouts, rows[i][5])
							}
						}
						if len(emptyOrderAccouts) > 0 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表账号" + joinListString(emptyOrderAccouts, "、") + "的案件编号是空的"))
							return
						}

						rowsHeader := handleRow(rows[0], mode)
						rowsBody := rows[1:]
						breakPoint := 0
						rowsBodyLen := len(rowsBody)

						// 按账号进行一次排序，保证相同账号的行挨在一起
						mergeSort(rowsBody, 0, rowsBodyLen-1, 5)

						for i, row := range rowsBody {
							if (i < rowsBodyLen-1 && row[20] != rowsBody[i+1][20]) || i == rowsBodyLen-1 {
								rowsBodySlice := [][]string{{}}
								if i == rowsBodyLen-1 {
									rowsBodySlice = rowsBody[breakPoint:]
								} else {
									rowsBodySlice = rowsBody[breakPoint : i+1]
								}
								breakPoint = i + 1
								order := rowsBodySlice[0][20]
								name := rowsBodySlice[0][19]
								templateOutput := [][]string{
									{"案件编号：", order, "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
									{"客户姓名：", name, "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
									{"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""},
								}
								for i := range rowsBodySlice {
									rowsBodySlice[i] = handleRow(rowsBodySlice[i], mode)
								}
								templateOutput = append(append(templateOutput, rowsHeader), rowsBodySlice...)

								// fBase, _ := excelize.OpenFile("/Users/huangqier/Documents/测试资料/test2-输出样板.xlsx")
								// sheetNameListBase := fBase.GetSheetMap()
								// var sheetNameBase string
								// for _, name := range sheetNameListBase {
								// 	sheetNameBase = name
								// }

								// 模板列宽和行高
								colWidthList := [19]float64{18.28, 12.37, 4.16, 8.75, 8.2, 8.41, 8.63, 10.41, 8.78, 15.34, 8.01, 15.15, 16.68, 9.53, 11.68, 9.48, 25.28, 19.01, 7.44}
								rowHeightList := [4]float64{9, 15}

								// for i := range colWidthList {
								// 	colName, _ := excelize.ColumnNumberToName(i + 1)
								// 	width, _ := fBase.GetColWidth(sheetNameBase, colName)
								// 	colWidthList[i] = width
								// }
								// for i := range rowHeightList {
								// 	height, _ := fBase.GetRowHeight(sheetNameBase, i+1) // 这里获取行高可能有bug
								// 	rowHeightList[i] = height
								// }

								// 模板打印格式
								var (
									marginBottom excelize.PageMarginBottom = 1
									marginFooter excelize.PageMarginFooter = 0.511805555555556
									marginHeader excelize.PageMarginHeader = 0.511805555555556
									marginLeft   excelize.PageMarginLeft   = 0.75
									marginRight  excelize.PageMarginRight  = 0.75
									marginTop    excelize.PageMarginTop    = 1
								)
								// fBase.GetPageMargins(sheetNameBase,
								// 	&marginBottom,
								// 	&marginFooter,
								// 	&marginHeader,
								// 	&marginLeft,
								// 	&marginRight,
								// 	&marginTop,
								// )

								// fmt.Println("colWidthList", colWidthList)
								// // fmt.Println("rowHeightList", rowHeightList)
								// fmt.Println("- marginBottom:", marginBottom)
								// fmt.Println("- marginFooter:", marginFooter)
								// fmt.Println("- marginHeader:", marginHeader)
								// fmt.Println("- marginLeft:", marginLeft)
								// fmt.Println("- marginRight:", marginRight)
								// fmt.Println("- marginTop:", marginTop)
								f := excelize.NewFile()
								sheetNameList := f.GetSheetMap()
								var sheetName string
								for _, name := range sheetNameList {
									sheetName = name
								}

								for i, row := range templateOutput {
									for j, cellValue := range row {
										cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
										f.SetCellValue(sheetName, cell, cellValue)
									}
								}

								rowLen := len(templateOutput)
								colLen := len(templateOutput[rowLen-1])
								lastCellAxis, _ := excelize.CoordinatesToCellName(colLen, rowLen)

								// 单元格格式
								styleBaseStr := `"alignment":{"horizontal":"center","vertical":"center"},"font":{"family":"宋体","size":10}`
								styleAlignmentLeftStr := `"alignment":{"horizontal":"left","vertical":"center"}`
								styleAlignmentJustifyStr := `"alignment":{"horizontal":"justify","vertical":"center","wrap_text":true}`

								styleBase, _ := f.NewStyle("{" + styleBaseStr + "}")
								err := f.SetCellStyle(sheetName, "A1", lastCellAxis, styleBase)

								styleAlignmentLeft, _ := f.NewStyle("{" + styleBaseStr + "," + styleAlignmentLeftStr + "}")
								err = f.SetCellStyle(sheetName, "B1", "B1", styleAlignmentLeft)
								err = f.SetCellStyle(sheetName, "B2", "B2", styleAlignmentLeft)

								styleAlignmentJustify, _ := f.NewStyle("{" + styleBaseStr + "," + styleAlignmentJustifyStr + "}")

								// 列宽和行高
								for i, colWidth := range colWidthList {
									colName, _ := excelize.ColumnNumberToName(i + 1)
									f.SetColWidth(sheetName, colName, colName, math.Trunc((colWidth*7+5)/7*256)/256)
								}
								for i := range templateOutput {
									if i == 2 {
										f.SetRowHeight(sheetName, i+1, rowHeightList[0])
									} else if i > 3 {
										err = f.SetCellStyle(sheetName, "Q"+strconv.Itoa(i+1), "Q"+strconv.Itoa(i+1), styleAlignmentJustify)
									} else {
										f.SetRowHeight(sheetName, i+1, rowHeightList[1])
									}
								}

								// 打印格式
								f.SetPageMargins(sheetName,
									excelize.PageMarginBottom(marginBottom),
									excelize.PageMarginFooter(marginFooter),
									excelize.PageMarginHeader(marginHeader),
									excelize.PageMarginLeft(marginLeft),
									excelize.PageMarginRight(marginRight),
									excelize.PageMarginTop(marginTop),
								)
								f.SetHeaderFooter(sheetName, &excelize.FormatHeaderFooter{
									DifferentFirst:   false,
									DifferentOddEven: false,
									OddHeader:        "",
									OddFooter:        "第 &P 页，共 &N 页",
									EvenHeader:       "",
									EvenFooter:       "第 &P 页，共 &N 页",
									FirstHeader:      "",
								})

								// 纸张横向
								f.SetPageLayout(sheetName,
									excelize.PageLayoutOrientation(excelize.OrientationLandscape),
									excelize.PageLayoutPaperSize(9),
									excelize.PageLayoutScale(54),
								)

								err = f.Save()

								path := folderPath + "/万用金明细数据/"
								fullPath := path + "/" + order + "万用金明细-" + name + ".xlsx"

								err = os.MkdirAll(path, os.ModePerm)
								if err != nil {
									fmt.Println(err)
									return
								}
								if err := f.SaveAs(fullPath); err != nil {
									fmt.Println(err)
									return
								}
								totalFileNum++
								conn.Write([]byte("成功生成第" + strconv.Itoa(totalFileNum) + "个文件"))
							}
						}
					} else if mode == "3" {
						// 进行字段数量校验
						row0Len := len(rows[0])
						if row0Len < 11 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表缺少了" + strconv.Itoa(11-row0Len) + "个列字段"))
							return
						}

						// 进行字段名字和顺序校验
						errNameList := []string{}
						keyNameList := []string{"账户号", "身份证号", "信用卡号", "分行代码", "日期", "时间", "手机号码", "发送状态", "短信内容", "客户姓名", "案件编号"}
						for i, name := range rows[0] {
							if name != keyNameList[i] {
								colName, _ := excelize.ColumnNumberToName(i + 1)
								errNameList = append(errNameList, "第 "+colName+` 列"`+name+`"应为"`+keyNameList[i]+`"`)
							}
						}
						if len(errNameList) > 0 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表列字段" + joinListString(errNameList, "、")))
							return
						}

						// 进行空行和案件编号校验
						emptyOrderAccouts := []string{}
						for i := rowsLen - 1; i >= 0; i-- {
							rowLen := len(rows[i])
							if rowLen == 0 || (rowLen > 0 && rows[i][0] == "") {
								rows = append(rows[:i], rows[i+1:]...)
							} else if (rows[i][10] == "" || rows[i][10] == "#N/A") && !isValueInList(rows[i][0], emptyOrderAccouts) {
								emptyOrderAccouts = append(emptyOrderAccouts, rows[i][0])
							}
						}
						if len(emptyOrderAccouts) > 0 {
							time.Sleep(time.Duration(10) * time.Millisecond)
							conn.Write([]byte("处理中断，因为第" + strconv.Itoa(sourceIndex+1) + "个文件的" + sheetName + "表账户号" + joinListString(emptyOrderAccouts, "、") + "的案件编号是空的"))
							return
						}

						rowsHeader := handleRow(rows[0], mode)
						rowsBody := rows[1:]
						breakPoint := 0
						rowsBodyLen := len(rowsBody)

						// 按账户号进行一次排序，保证相同账户号的行挨在一起
						mergeSort(rowsBody, 0, rowsBodyLen-1, 0)

						for i, row := range rowsBody {
							if (i < rowsBodyLen-1 && row[10] != rowsBody[i+1][10]) || i == rowsBodyLen-1 {
								rowsBodySlice := [][]string{{}}
								if i == rowsBodyLen-1 {
									rowsBodySlice = rowsBody[breakPoint:]
								} else {
									rowsBodySlice = rowsBody[breakPoint : i+1]
								}
								breakPoint = i + 1
								order := rowsBodySlice[0][10]
								name := rowsBodySlice[0][9]
								templateOutput := [][]string{
									{"案件编号：", order, "", "", "", "", "", "", ""},
									{"客户姓名：", name, "", "", "", "", "", "", ""},
									{"", "", "", "", "", "", "", "", ""},
								}
								for i := range rowsBodySlice {
									rowsBodySlice[i] = handleRow(rowsBodySlice[i], mode)
								}
								templateOutput = append(append(templateOutput, rowsHeader), rowsBodySlice...)

								// fBase, _ := excelize.OpenFile("/Users/huangqier/Documents/测试资料/test3-输出样板.xlsx")
								// sheetNameListBase := fBase.GetSheetMap()
								// var sheetNameBase string
								// for _, name := range sheetNameListBase {
								// 	sheetNameBase = name
								// }

								// 模板列宽和行高
								colWidthList := [9]float64{8.51, 17.13, 15.2, 7.82, 8.51, 7.01, 10.26, 8.7, 58.67}
								rowHeightList := [6]float64{9, 15}

								// for i := range colWidthList {
								// 	colName, _ := excelize.ColumnNumberToName(i + 1)
								// 	width, _ := fBase.GetColWidth(sheetNameBase, colName)
								// 	colWidthList[i] = width
								// }
								// for i := range rowHeightList {
								// 	height, _ := fBase.GetRowHeight(sheetNameBase, i+1) // 这里获取行高可能有bug
								// 	rowHeightList[i] = height
								// }

								// 模板打印格式
								var (
									marginBottom excelize.PageMarginBottom = 1
									marginFooter excelize.PageMarginFooter = 0.511805555555556
									marginHeader excelize.PageMarginHeader = 0.511805555555556
									marginLeft   excelize.PageMarginLeft   = 0.75
									marginRight  excelize.PageMarginRight  = 0.75
									marginTop    excelize.PageMarginTop    = 1
								)
								// fBase.GetPageMargins(sheetNameBase,
								// 	&marginBottom,
								// 	&marginFooter,
								// 	&marginHeader,
								// 	&marginLeft,
								// 	&marginRight,
								// 	&marginTop,
								// )

								// fmt.Println("colWidthList", colWidthList)
								// // fmt.Println("rowHeightList", rowHeightList)
								// fmt.Println("- marginBottom:", marginBottom)
								// fmt.Println("- marginFooter:", marginFooter)
								// fmt.Println("- marginHeader:", marginHeader)
								// fmt.Println("- marginLeft:", marginLeft)
								// fmt.Println("- marginRight:", marginRight)
								// fmt.Println("- marginTop:", marginTop)

								f := excelize.NewFile()
								sheetNameList := f.GetSheetMap()
								var sheetName string
								for _, name := range sheetNameList {
									sheetName = name
								}

								for i, row := range templateOutput {
									for j, cellValue := range row {
										cell, _ := excelize.CoordinatesToCellName(j+1, i+1)
										f.SetCellValue(sheetName, cell, cellValue)
									}
								}

								rowLen := len(templateOutput)
								colLen := len(templateOutput[rowLen-1])
								lastCellAxis, _ := excelize.CoordinatesToCellName(colLen, rowLen)

								// 单元格格式
								styleBaseStr := `"alignment":{"horizontal":"center","vertical":"center"},"font":{"family":"宋体","size":10}`
								styleAlignmentLeftStr := `"alignment":{"horizontal":"left","vertical":"center"}`
								styleAlignmentJustifyStr := `"alignment":{"horizontal":"justify","vertical":"center","wrap_text":true}`

								styleBase, _ := f.NewStyle("{" + styleBaseStr + "}")
								err := f.SetCellStyle(sheetName, "A1", lastCellAxis, styleBase)

								styleAlignmentLeft, _ := f.NewStyle("{" + styleBaseStr + "," + styleAlignmentLeftStr + "}")
								err = f.SetCellStyle(sheetName, "B1", "B1", styleAlignmentLeft)
								err = f.SetCellStyle(sheetName, "B2", "B2", styleAlignmentLeft)

								styleAlignmentJustify, _ := f.NewStyle("{" + styleBaseStr + "," + styleAlignmentJustifyStr + "}")

								// 列宽和行高
								for i, colWidth := range colWidthList {
									colName, _ := excelize.ColumnNumberToName(i + 1)
									f.SetColWidth(sheetName, colName, colName, math.Trunc((colWidth*7+5)/7*256)/256)
								}
								for i := range templateOutput {
									if i == 2 {
										f.SetRowHeight(sheetName, i+1, rowHeightList[0])
									} else if i > 3 {
										err = f.SetCellStyle(sheetName, "I"+strconv.Itoa(i+1), "I"+strconv.Itoa(i+1), styleAlignmentJustify)
									} else {
										f.SetRowHeight(sheetName, i+1, rowHeightList[1])
									}
								}

								// 打印格式
								f.SetPageMargins(sheetName,
									excelize.PageMarginBottom(marginBottom),
									excelize.PageMarginFooter(marginFooter),
									excelize.PageMarginHeader(marginHeader),
									excelize.PageMarginLeft(marginLeft),
									excelize.PageMarginRight(marginRight),
									excelize.PageMarginTop(marginTop),
								)
								f.SetHeaderFooter(sheetName, &excelize.FormatHeaderFooter{
									DifferentFirst:   false,
									DifferentOddEven: false,
									OddHeader:        "",
									OddFooter:        "第 &P 页，共 &N 页",
									EvenHeader:       "",
									EvenFooter:       "第 &P 页，共 &N 页",
									FirstHeader:      "",
								})

								// 纸张横向
								f.SetPageLayout(sheetName,
									excelize.PageLayoutOrientation(excelize.OrientationLandscape),
									excelize.PageLayoutPaperSize(9),
									excelize.PageLayoutScale(87),
								)

								err = f.Save()

								path := folderPath + "/万用金短信/"
								fullPath := path + "/" + order + "万用金短信-" + name + ".xlsx"

								err = os.MkdirAll(path, os.ModePerm)
								if err != nil {
									fmt.Println(err)
									return
								}
								if err := f.SaveAs(fullPath); err != nil {
									fmt.Println(err)
									return
								}
								totalFileNum++
								conn.Write([]byte("成功生成第" + strconv.Itoa(totalFileNum) + "个文件"))
							}
						}
					}
				}
			}
			time.Sleep(time.Duration(10) * time.Millisecond)
			conn.Write([]byte("处理结束！花费时间: " + strconv.FormatInt(time.Now().Unix()-startTime, 10) + "秒，大约" + strconv.FormatFloat(round(float64((time.Now().Unix()-startTime))/60, 2), 'f', -1, 64) + "分钟，共生成" + strconv.Itoa(totalFileNum) + "个文件"))
			fmt.Println("处理完毕，花费时间: ", (time.Now().Unix() - startTime), "秒，大约", round(float64((time.Now().Unix()-startTime))/60, 2), "分钟，共生成", totalFileNum, "个文件")

			// // 以下为处理好数据返回给node生成文件
			// startTime := time.Now().Unix()
			// // 文件包 []
			// // 多个文件 []
			// // 多个工作表 []
			// // 多行 []
			// // filesData := [][][][]string{}
			// sheetList := [][][]string{}
			// for _, path := range sourcePathList {
			// 	// filesData = append(filesData, parseFile(path))
			// 	for _, sheet := range parseFile(path) {
			// 		sheetList = append(sheetList, sheet)
			// 	}
			// }
			// // sheetList := [][][]string{}
			// // for _, file := range filesData {
			// // 	for _, sheet := range file {
			// // 		sheetList = append(sheetList, sheet)
			// // 	}
			// // }

			// data, err := json.Marshal(sheetList) // 转成JSON字符串
			// if err != nil {
			// 	fmt.Println(err)
			// 	return
			// }
			// fmt.Println("解析文件花费时间: ", (time.Now().Unix()-startTime)/60)
			// startTime = time.Now().Unix()
			// fmt.Println("开始写返回数据")
			// conn.Write([]byte(string(data) + "数据传输结束标记"))
			// fmt.Println("传输数据花费时间: ", (time.Now().Unix()-startTime)/60)
		} else {
			// 如果是node生成文件则会有第二次通信，第二次通信会传生成文件所在的文件夹路径过来
			folderPath := string(tcpData)

			// fBase, _ := excelize.OpenFile("/Users/huangqier/Downloads/node成品模板.xlsx")
			// sheetNameListBase := fBase.GetSheetMap()
			// var sheetNameBase string
			// for _, name := range sheetNameListBase {
			// 	sheetNameBase = name
			// }

			// 模板列宽和行高
			colWidthList := [6]float64{11.2589285714286, 9.64285714285714, 9.98214285714286, 27.6339285714286, 12.4285714285714, 9.64285714285714}
			rowHeightList := [2]float64{10.8, 20.4}
			// for i := range colWidthList {
			// 	colName, _ := excelize.ColumnNumberToName(i + 1)
			// 	width, _ := fBase.GetColWidth(sheetNameBase, colName)
			// 	colWidthList[i] = width
			// }
			// for i := range rowHeightList {
			// 	height, _ := fBase.GetRowHeight(sheetNameBase, i+1) // 这里获取行高有bug，所以先取固定值
			// 	rowHeightList[i] = height
			// }

			// 模板打印格式
			var (
				marginBottom excelize.PageMarginBottom = 0.313888888888889
				marginFooter excelize.PageMarginFooter = 0.118055555555556
				marginHeader excelize.PageMarginHeader = 0.118055555555556
				marginLeft   excelize.PageMarginLeft   = 0.751388888888889
				marginRight  excelize.PageMarginRight  = 0.590277777777778
				marginTop    excelize.PageMarginTop    = 0.313888888888889
			)
			// fBase.GetPageMargins(sheetNameBase,
			// 	&marginBottom,
			// 	&marginFooter,
			// 	&marginHeader,
			// 	&marginLeft,
			// 	&marginRight,
			// 	&marginTop,
			// )

			// fmt.Println("colWidthList", colWidthList)
			// fmt.Println("- marginBottom:", marginBottom)
			// fmt.Println("- marginFooter:", marginFooter)
			// fmt.Println("- marginHeader:", marginHeader)
			// fmt.Println("- marginLeft:", marginLeft)
			// fmt.Println("- marginRight:", marginRight)
			// fmt.Println("- marginTop:", marginTop)

			var fileList []string
			getAllFile(folderPath, &fileList)
			fileListLen := len(fileList)
			conn.Write([]byte("正在批量处理excel文件格式...0" + "/" + strconv.Itoa(fileListLen)))
			for fileIndex, filePath := range fileList {
				f, err := excelize.OpenFile(filePath)
				if err != nil {
					fmt.Println(err)
					return
				}
				sheetNameMap := f.GetSheetMap()
				var sheetName string
				for _, name := range sheetNameMap {
					sheetName = name
				}
				rows, _ := f.GetRows(sheetName)
				rowLen := len(rows)
				colLen := len(rows[rowLen-1])
				lastCellAxis, _ := excelize.CoordinatesToCellName(colLen, rowLen)
				lastDateCellAxis, _ := excelize.CoordinatesToCellName(2, rowLen)

				// 列宽和行高
				for i, colWidth := range colWidthList {
					colName, _ := excelize.ColumnNumberToName(i + 1)
					f.SetColWidth(sheetName, colName, colName, colWidth)
				}
				for i := range rows {
					if i == 1 {
						f.SetRowHeight(sheetName, i+1, rowHeightList[1])
					} else {
						f.SetRowHeight(sheetName, i+1, rowHeightList[0])
					}
				}

				// 单元格格式
				styleBaseStr := `"alignment":{"horizontal":"center","vertical":"center"},"font":{"family":"宋体","size":7}`
				styleBorderStr := `"border":[{"type":"left","color":"000000","style":1},{"type":"top","color":"000000","style":1},{"type":"bottom","color":"000000","style":1},{"type":"right","color":"000000","style":1}]`
				styleDateStr := `"number_format":14`
				styleFontSizeStr := `"font":{"family":"宋体","size":14}`

				styleBase, _ := f.NewStyle("{" + styleBaseStr + "}")
				err = f.SetCellStyle(sheetName, "A1", lastCellAxis, styleBase)

				styleBorder, _ := f.NewStyle("{" + styleBaseStr + "," + styleBorderStr + "}")
				err = f.SetCellStyle(sheetName, "A5", lastCellAxis, styleBorder)

				styleDate, _ := f.NewStyle("{" + styleBaseStr + "," + styleBorderStr + "," + styleDateStr + "}")
				err = f.SetCellStyle(sheetName, "B6", lastDateCellAxis, styleDate)

				styleFontSize, _ := f.NewStyle("{" + styleBaseStr + "," + styleFontSizeStr + "}")
				err = f.SetCellStyle(sheetName, "A2", "A2", styleFontSize)

				f.SetCellValue(sheetName, "D3", "账  户  号：")
				f.MergeCell(sheetName, "A1", "F1")
				f.MergeCell(sheetName, "A2", "F2")

				// 打印格式
				f.SetPageMargins(sheetName,
					excelize.PageMarginBottom(marginBottom),
					excelize.PageMarginFooter(marginFooter),
					excelize.PageMarginHeader(marginHeader),
					excelize.PageMarginLeft(marginLeft),
					excelize.PageMarginRight(marginRight),
					excelize.PageMarginTop(marginTop),
				)
				f.SetHeaderFooter(sheetName, &excelize.FormatHeaderFooter{
					DifferentFirst:   false,
					DifferentOddEven: false,
					OddHeader:        "",
					OddFooter:        "第 &P 页，共 &N 页",
					EvenHeader:       "",
					EvenFooter:       "第 &P 页，共 &N 页",
					FirstHeader:      "",
				})

				err = f.Save()
				if fileIndex+1 < fileListLen {
					conn.Write([]byte("正在批量处理excel文件格式..." + strconv.Itoa(fileIndex+1) + "/" + strconv.Itoa(fileListLen)))
				} else {
					conn.Write([]byte("处理结束！共生成了" + strconv.Itoa(fileListLen) + "个文件"))
				}
			}
		}

		// Log(conn.RemoteAddr().String(), "receive data string:\n", string(buffer[:n]))

	}
}

// 按账户号进行归并排序（因为又快又有稳定性）, tIndex是账户号的index
func mergeSort(arr [][]string, l int, r int, tIndex int) {
	if l >= r {
		return
	}
	// 递归向下
	mid := (r + l) / 2
	mergeSort(arr, l, mid, tIndex)
	mergeSort(arr, mid+1, r, tIndex)
	// 归并向上
	merge(arr, l, mid, r, tIndex)
}

// 合并 [l,r] 两部分数据，mid 左半部分的终点，mid + 1 是右半部分的起点
func merge(arr [][]string, l int, mid int, r int, tIndex int) {
	// 因为需要直接修改 arr 数据，这里首先复制 [l,r] 的数据到新的数组中，用于赋值操作
	temp := make([][]string, r-l+1)
	for i := l; i <= r; i++ {
		temp[i-l] = arr[i]
	}
	// 指向两部分起点
	left := l
	right := mid + 1
	for i := l; i <= r; i++ {
		// 左边的点超过中点，说明只剩右边的数据
		if left > mid {
			arr[i] = temp[right-l]
			right++
			// 右边的数据超过终点，说明只剩左边的数据
		} else if right > r {
			arr[i] = temp[left-l]
			left++
			// 左边的数据大于右边的数据，选小的
		} else if temp[left-l][tIndex] > temp[right-l][tIndex] {
			arr[i] = temp[right-l]
			right++
		} else {
			arr[i] = temp[left-l]
			left++
		}
	}
}

func joinListString(list []string, s string) string {
	var listStr string
	listLen := len(list)
	if listLen == 0 {
		return ""
	}
	for i, str := range list {
		if i < listLen-1 {
			listStr += str + s
		} else {
			listStr += str
		}
	}
	return listStr
}

func round(f float64, n int) float64 {
	n10 := math.Pow10(n)
	return math.Trunc((f+0.5/n10)*n10) / n10
}

func isValueInList(value string, list []string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

func handleRow(row []string, mode string) []string {
	if mode == "1" {
		// 删掉不需要的列
		deleteList := []int{9, 8, 7, 4}
		if len(row) == 11 {
			deleteList = []int{10, 9, 8, 7, 4}
		}
		for _, i := range deleteList {
			row = append(row[:i], row[i+1:]...)
		}
		// 调换列的位置
		item := row[1]
		row[1] = row[3]
		row[3] = item
		item = row[2]
		row[2] = row[4]
		row[4] = item
	} else if mode == "2" {
		// 删掉不需要的列
		deleteList := []int{20, 19}
		for _, i := range deleteList {
			row = append(row[:i], row[i+1:]...)
		}
	} else if mode == "3" {
		// 删掉不需要的列
		deleteList := []int{10, 9}
		for _, i := range deleteList {
			row = append(row[:i], row[i+1:]...)
		}
	}
	return row
}

func parseFile(path string) [][][]string {
	f, err := excelize.OpenFile(path)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sheetMap := f.GetSheetMap()
	rowsData := [][][]string{}
	for _, sheetName := range sheetMap {
		rows, _ := f.GetRows(sheetName)
		rowsData = append(rowsData, rows)
	}
	return rowsData
}

// Log 输出
func Log(v ...interface{}) {
	log.Println(v...)
}

// CheckError 处理error
func CheckError(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal error: %s", err.Error())
		os.Exit(1)
	}
}

func getAllFile(pathname string, fileList *[]string) error {
	rd, err := ioutil.ReadDir(pathname)
	for _, fi := range rd {
		if fi.IsDir() {
			getAllFile(pathname+"/"+fi.Name(), fileList)
		} else {
			if path.Ext(fi.Name()) == ".xlsx" {
				*fileList = append(*fileList, pathname+"/"+fi.Name())
			}
		}
	}
	return err
}
