package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"octochan/config"
	"octochan/report"
	"octochan/scenario"
	"octochan/workspace"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	requestDoc     string
	requestComment string
	rejectReason   string
	applyRequestID string
	notifyMail     bool
)

// requestStore открывает хранилище заявок.
func requestStore() (*workspace.Requests, error) {
	fm, err := mustFileManager()
	if err != nil {
		return nil, err
	}
	return workspace.NewRequests(fm.RequestsDir())
}

var requestCmd = &cobra.Command{
	Use:   "request <сценарий> | request <база> <изменения>",
	Short: "Создать заявку на изменение из плана",
	Long: "Заявка фиксирует план и ждёт решения. Выполнить её можно только\n" +
		"после согласования: ochan apply --request <id>.\n\n" +
		"Заявка отвечает не на вопрос «кому в принципе можно», а на вопрос\n" +
		"«на это конкретное изменение сказали да» — именно это нужно аудиту.",
	Args: cobra.MinimumNArgs(1),
	Example: "ochan request restart.yaml --doc заявка.docx\n" +
		"ochan request prod.conf changes.conf",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := requestStore()
		if err != nil {
			return err
		}

		request, err := buildRequest(cmd, args)
		if err != nil {
			return err
		}
		if err := store.Create(request); err != nil {
			return err
		}

		cmd.Printf("Заявка %s создана (%s)\n", request.ID, request.State.Russian())
		cmd.Printf("Изменений: %d\n", request.Plan.Len())

		if requestDoc != "" {
			if err := writeRequestDoc(request, requestDoc); err != nil {
				return err
			}
			cmd.Printf("Документ: %s\n", requestDoc)
		}

		cmd.Printf("Согласовать: ochan approve %s\n", request.ID)
		return nil
	},
}

// buildRequest строит заявку из сценария или из пары конфигов.
func buildRequest(cmd *cobra.Command, args []string) (*workspace.Request, error) {
	if len(args) >= 2 && !isOverride(args[1]) {
		return configRequest(cmd, args)
	}
	return scenarioRequest(cmd, args)
}

func scenarioRequest(cmd *cobra.Command, args []string) (*workspace.Request, error) {
	target, overrides, err := scenarioArgs(args)
	if err != nil {
		return nil, err
	}

	src, err := readSource(target)
	if err != nil {
		return nil, err
	}

	parsed, err := parseScenario(src)
	if err != nil {
		return nil, err
	}
	parsed = parsed.WithOverrides(overrides)

	runner, err := scenario.Resolve(parsed.Service)
	if err != nil {
		return nil, err
	}

	plan, err := runner.Plan(cmd.Context(), parsed)
	if err != nil {
		return nil, err
	}

	// Текст сценария сохраняется в заявке, чтобы выполнить ровно то,
	// что согласовали. Перечитывать файл с диска нельзя: между
	// согласованием и выполнением его могли поправить.
	return &workspace.Request{
		Source:  src.Name,
		Service: parsed.Service,
		Summary: fmt.Sprintf("%s через %s, операций: %d", parsed.Service, runner.Kind(), plan.Len()),
		Plan:    plan,
		Payload: src.Content,
	}, nil
}

func configRequest(cmd *cobra.Command, args []string) (*workspace.Request, error) {
	ctx := cmd.Context()
	opts := config.SourceOptions{}

	base, err := loadConfigArg(ctx, args[0], opts)
	if err != nil {
		return nil, err
	}
	changes, err := loadConfigArg(ctx, args[1], opts)
	if err != nil {
		return nil, err
	}

	diff := config.CompareConfigs(base.Data, changes.Data, base.Name, changes.Name)
	plan := config.PlanFromDiff(diff, base.Name)
	plan.Command = "patch"
	plan.Source = changes.Name
	plan.Warnings = append(plan.Warnings, base.Notes...)
	plan.Warnings = append(plan.Warnings, changes.Notes...)

	return &workspace.Request{
		Source:  changes.Name,
		Target:  base.Name,
		Summary: fmt.Sprintf("правка %s, параметров: %d", base.Name, plan.Len()),
		Plan:    plan,
	}, nil
}

func writeRequestDoc(request *workspace.Request, path string) error {
	doc := report.Document{
		Title:   "Заявка на изменение " + request.ID,
		Author:  request.Author,
		Created: request.Created,
		Plan:    request.Plan,
		Fields: []report.Field{
			{Name: "Идентификатор", Value: request.ID},
			{Name: "Состояние", Value: request.State.Russian()},
			{Name: "Автор", Value: request.Author},
			{Name: "Создана", Value: request.Created.Local().Format("2006-01-02 15:04:05")},
			{Name: "Источник", Value: request.Source},
			{Name: "Итог", Value: request.Summary},
		},
	}
	if request.Service != "" {
		doc.Fields = append(doc.Fields, report.Field{Name: "Сервис", Value: request.Service})
	}
	if request.Target != "" {
		doc.Fields = append(doc.Fields, report.Field{Name: "Цель", Value: request.Target})
	}
	if request.DecidedBy != "" {
		doc.Fields = append(doc.Fields,
			report.Field{Name: "Решение", Value: request.State.Russian() + " — " + request.DecidedBy})
		if request.Decision != "" {
			doc.Comments = append(doc.Comments, request.Decision)
		}
	}

	data, err := report.DOCX(doc)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("не удалось записать документ %s: %w", path, err)
	}
	return nil
}

var requestsCmd = &cobra.Command{
	Use:   "requests [id]",
	Short: "Показать заявки",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := requestStore()
		if err != nil {
			return err
		}

		if len(args) == 1 {
			request, err := store.Get(args[0])
			if err != nil {
				return err
			}
			return showRequest(cmd, request)
		}

		all, err := store.List()
		if err != nil {
			return err
		}
		if len(all) == 0 {
			cmd.Printf("Заявок нет. Хранилище: %s\n", store.Dir())
			return nil
		}

		for _, request := range all {
			cmd.Println(request.Describe())
		}
		return nil
	},
}

func showRequest(cmd *cobra.Command, request *workspace.Request) error {
	cmd.Printf("Заявка:    %s\n", request.ID)
	cmd.Printf("Состояние: %s\n", request.State.Russian())
	cmd.Printf("Автор:     %s\n", request.Author)
	cmd.Printf("Создана:   %s\n", request.Created.Local().Format("2006-01-02 15:04:05"))
	cmd.Printf("Источник:  %s\n", request.Source)
	if request.Service != "" {
		cmd.Printf("Сервис:    %s\n", request.Service)
	}
	if request.Target != "" {
		cmd.Printf("Цель:      %s\n", request.Target)
	}
	cmd.Printf("Итог:      %s\n", request.Summary)

	if request.DecidedBy != "" {
		cmd.Printf("Решение:   %s, %s (%s)\n",
			request.State.Russian(), request.DecidedBy,
			request.DecidedAt.Local().Format("2006-01-02 15:04:05"))
		if request.Decision != "" {
			cmd.Printf("Коммент.:  %s\n", request.Decision)
		}
	}
	if !request.AppliedAt.IsZero() {
		cmd.Printf("Выполнена: %s\n", request.AppliedAt.Local().Format("2006-01-02 15:04:05"))
	}

	if request.Plan != nil {
		cmd.Print("\n" + request.Plan.Render(60))
	}

	if requestDoc != "" {
		if err := writeRequestDoc(request, requestDoc); err != nil {
			return err
		}
		cmd.Printf("\nДокумент: %s\n", requestDoc)
	}
	return nil
}

var approveCmd = &cobra.Command{
	Use:   "approve <id>",
	Short: "Согласовать заявку",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return decideRequest(cmd, args[0], workspace.RequestApproved, requestComment)
	},
}

var rejectCmd = &cobra.Command{
	Use:   "reject <id>",
	Short: "Отклонить заявку",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if rejectReason == "" {
			return fmt.Errorf("укажите причину отказа: --reason")
		}
		return decideRequest(cmd, args[0], workspace.RequestRejected, rejectReason)
	},
}

func decideRequest(cmd *cobra.Command, id string, state workspace.RequestState, comment string) error {
	store, err := requestStore()
	if err != nil {
		return err
	}

	request, err := store.Get(id)
	if err != nil {
		return err
	}
	if err := store.Decide(request, state, "", comment); err != nil {
		return err
	}

	cmd.Printf("Заявка %s: %s (%s)\n", request.ID, request.State.Russian(), request.DecidedBy)
	if state == workspace.RequestApproved {
		cmd.Printf("Выполнить: ochan apply --request %s\n", request.ID)
	}

	notify(cmd, fmt.Sprintf("Заявка %s %s", request.ID, request.State.Russian()),
		requestMailBody(request), nil)
	return nil
}

func requestMailBody(request *workspace.Request) string {
	body := fmt.Sprintf("Заявка: %s\nСостояние: %s\nАвтор: %s\nИсточник: %s\nИтог: %s\n",
		request.ID, request.State.Russian(), request.Author, request.Source, request.Summary)
	if request.DecidedBy != "" {
		body += fmt.Sprintf("Решение принял: %s\n", request.DecidedBy)
	}
	if request.Decision != "" {
		body += fmt.Sprintf("Комментарий: %s\n", request.Decision)
	}
	if request.Plan != nil {
		body += "\n" + request.Plan.Render(60)
	}
	return body
}

// notify отправляет письмо, если почта настроена.
//
// Ненастроенная почта — не ошибка: письмо просто не нужно. А вот сбой
// отправки при настроенной почте показывается, но работу не прерывает:
// изменение уже сделано, и откатывать его из-за письма неправильно.
func notify(cmd *cobra.Command, subject, body string, attachments []report.Attachment) {
	if !notifyMail && !viper.GetBool("defaults.notify_always") {
		return
	}

	cfg := report.MailConfig{
		Host:     viper.GetString("defaults.smtp_host"),
		Port:     viper.GetInt("defaults.smtp_port"),
		From:     viper.GetString("defaults.smtp_from"),
		To:       viper.GetStringSlice("defaults.smtp_to"),
		Username: viper.GetString("defaults.smtp_user"),
		Password: os.Getenv("OCTOCHAN_SMTP_PASSWORD"),
		StartTLS: viper.GetBool("defaults.smtp_starttls"),
	}

	err := report.Send(cfg, report.Mail{Subject: subject, Body: body, Attachments: attachments})
	switch {
	case err == nil:
		cmd.Printf("Уведомление отправлено: %s\n", strings.Join(cfg.To, ", "))
	case isNotConfigured(err):
		cmd.PrintErrf("⚠️ уведомление не отправлено: %v\n", err)
	default:
		cmd.PrintErrf("⚠️ не удалось отправить уведомление: %v\n", err)
	}
}

// isNotConfigured отличает «почта не настроена» от сбоя отправки.
func isNotConfigured(err error) bool { return errors.Is(err, report.ErrMailNotConfigured) }

func init() {
	requestCmd.Flags().StringVar(&requestDoc, "doc", "", "выгрузить заявку в файл .docx")
	requestsCmd.Flags().StringVar(&requestDoc, "doc", "", "выгрузить заявку в файл .docx")
	approveCmd.Flags().StringVar(&requestComment, "comment", "", "комментарий к решению")
	rejectCmd.Flags().StringVar(&rejectReason, "reason", "", "причина отказа")
	applyCmd.Flags().StringVar(&applyRequestID, "request", "", "выполнить согласованную заявку")
	applyCmd.Flags().BoolVar(&notifyMail, "notify", false, "отправить уведомление по почте")
	approveCmd.Flags().BoolVar(&notifyMail, "notify", false, "отправить уведомление по почте")
	rejectCmd.Flags().BoolVar(&notifyMail, "notify", false, "отправить уведомление по почте")
}
